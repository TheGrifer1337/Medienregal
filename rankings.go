package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type annualChart struct {
	Year   int     `json:"year"`
	Source string  `json:"source"`
	Items  []entry `json:"items"`
}

type chartCache struct {
	Chart annualChart
	Until time.Time
}

var annualCharts sync.Map

func (a *app) rankings(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	if _, ok := statuses[kind]; !ok {
		fail(w, 400, "Unbekannte Medienkategorie.")
		return
	}
	genre := r.URL.Query().Get("genre")
	if genre != "" && chartGenres[kind][genre].term == "" {
		fail(w, 400, "Unbekanntes Chart-Genre.")
		return
	}
	year := time.Now().Year()
	token := a.getSettings().TMDB
	key := struct {
		kind, genre string
		year        int
		token       string
	}{kind, genre, year, token}
	if val, ok := annualCharts.Load(key); ok {
		cached := val.(chartCache)
		if time.Now().Before(cached.Until) {
			reply(w, 200, cached.Chart)
			return
		}
	}
	chart, err := loadChart(r.Context(), kind, year, token, genre)
	if err != nil {
		fail(w, 502, "Die Jahresliste ist gerade nicht verfügbar.")
		return
	}
	annualCharts.Store(key, chartCache{Chart: chart, Until: time.Now().Add(40 * time.Minute)})
	reply(w, 200, chart)
}

func loadChart(ctx context.Context, kind string, year int, token, genre string) (annualChart, error) {
	if kind == "book" {
		return bookChart(ctx, year, genre)
	}
	if kind == "game" {
		chart, err := wikidataChart(ctx, kind, year, genre)
		if err == nil && len(chart.Items) == 50 {
			return chart, nil
		}
		fallback, fallbackErr := steamChart(ctx, year, genre)
		if fallbackErr == nil && (err != nil || len(fallback.Items) > len(chart.Items)) {
			return fallback, nil
		}
		return chart, err
	}
	if token != "" && (genre == "" || chartGenres[kind][genre].tmdb != "") {
		if chart, err := tmdbChart(ctx, kind, year, token, genre); err == nil && len(chart.Items) > 0 {
			return chart, nil
		}
	}
	if genre != "" {
		result, err := wikidataChart(ctx, kind, year, genre)
		if err == nil && len(result.Items) == 50 {
			return result, nil
		}
		if other, otherErr := cinemetaChart(ctx, kind, year, genre); otherErr == nil && (err != nil || len(other.Items) > len(result.Items)) {
			result, err = other, nil
			if len(result.Items) == 50 {
				return result, nil
			}
		}
		if other, otherErr := imdbChart(ctx, kind, year, genre); otherErr == nil && (err != nil || len(other.Items) > len(result.Items)) {
			return other, nil
		}
		return result, err
	}
	result, err := cinemetaChart(ctx, kind, year, "")
	if err == nil && len(result.Items) == 50 {
		return result, nil
	}
	fallback, fallbackErr := wikidataChart(ctx, kind, year, "")
	if fallbackErr == nil && len(fallback.Items) > len(result.Items) {
		return fallback, nil
	}
	if len(result.Items) == 0 {
		if fallback, fallbackErr = imdbChart(ctx, kind, year, ""); fallbackErr == nil && len(fallback.Items) > 0 {
			return fallback, nil
		}
	}
	return result, err
}

func bookChart(ctx context.Context, year int, genre string) (annualChart, error) {
	query := "first_publish_year:" + strconv.Itoa(year)
	if genre != "" {
		query += " AND subject:" + strconv.Quote(chartGenres["book"][genre].term)
	}
	params := "q=" + url.QueryEscape(query) + "&sort=readinglog&limit=50&fields=key,title,author_name,cover_i,readinglog_count,first_publish_year"
	var payload struct {
		Docs []struct {
			Key, Title string
			Authors    []string `json:"author_name"`
			Cover      int      `json:"cover_i"`
			Pages      int      `json:"number_of_pages_median"`
			Subjects   []string `json:"subject"`
		}
	}
	chart := annualChart{Year: year, Source: "Open Library · Leselisten", Items: []entry{}}
	err := getJSON(ctx, "https://openlibrary.org/search.json?"+params, "", &payload)
	for _, book := range payload.Docs {
		if !openLibraryWork.MatchString(book.Key) || book.Title == "" {
			continue
		}
		chart.Items = append(chart.Items, entry{ID: "book:open:" + book.Key, Kind: "book", Title: book.Title,
			Subtitle: strings.Join(book.Authors, ", "), Cover: bookCover(book.Cover), Genres: classifyBookGenres(book.Subjects),
			Total: book.Pages, Source: "Open Library"})
	}
	return chart, err
}

func tmdbChart(ctx context.Context, kind string, year int, token, genre string) (annualChart, error) {
	chart := annualChart{Year: year, Source: "TMDB · Beliebtheit", Items: []entry{}}
	q := url.Values{"language": {"de-DE"}, "sort_by": {"popularity.desc"}, "include_adult": {"false"}}
	if kind == "movie" {
		q.Set("primary_release_year", strconv.Itoa(year))
		q.Set("primary_release_date.lte", time.Now().Format("2006-01-02"))
	} else {
		q.Set("first_air_date_year", strconv.Itoa(year))
		q.Set("first_air_date.lte", time.Now().Format("2006-01-02"))
	}
	if id := chartGenres[kind][genre].tmdb; id != "" {
		q.Set("with_genres", id)
	}
	seen := map[int]bool{}
	for page := 1; page <= 3 && len(chart.Items) < 50; page++ {
		q.Set("page", strconv.Itoa(page))
		var payload struct {
			Results    []tmdbTitle
			TotalPages int `json:"total_pages"`
		}
		err := getJSON(ctx, "https://api.themoviedb.org/3/discover/"+tmdbKind(kind)+"?"+q.Encode(), token, &payload)
		if err != nil {
			return chart, err
		}
		for _, title := range payload.Results {
			if title.ID <= 0 || seen[title.ID] {
				continue
			}
			item := title.internal(kind)
			if item.Title == "" {
				continue
			}
			seen[title.ID] = true
			chart.Items = append(chart.Items, item)
			if len(chart.Items) == 50 {
				break
			}
		}
		if page >= payload.TotalPages {
			break
		}
	}
	return chart, nil
}

func cinemetaChart(ctx context.Context, kind string, year int, genre string) (annualChart, error) {
	chart := annualChart{Year: year, Source: "Cinemeta · Jahreskatalog", Items: []entry{}}
	catalog, maxOffset := "year", 100
	yearLabel := strconv.Itoa(year)
	if genre != "" {
		catalog, maxOffset = "top", 400
		chart.Source = "Cinemeta · Beliebtheit"
	}
	seen := map[string]bool{}
	for offset := 0; offset < maxOffset && len(chart.Items) < 50; {
		var payload struct{ Metas []cinemeta }
		filter := "genre=" + yearLabel
		if genre != "" {
			filter = "genre=" + url.PathEscape(chartGenres[kind][genre].term)
		}
		if offset > 0 {
			filter += fmt.Sprintf("&skip=%d", offset)
		}
		if err := getJSON(ctx, "https://v3-cinemeta.strem.io/catalog/"+kind+"/"+catalog+"/"+filter+".json", "", &payload); err != nil {
			if len(chart.Items) > 0 {
				return chart, nil
			}
			return chart, err
		}
		newTitles := 0
		for _, title := range payload.Metas {
			if !imdbID.MatchString(title.ID) || seen[title.ID] || title.Name == "" {
				continue
			}
			seen[title.ID] = true
			newTitles++
			if genre != "" && !strings.HasPrefix(title.ReleaseInfo, yearLabel) {
				continue
			}
			item := title.internal(kind)
			if genre != "" {
				item.Genres = mergeLabels(item.Genres, []string{genre})
			}
			chart.Items = append(chart.Items, item)
			if len(chart.Items) == 50 {
				break
			}
		}
		if newTitles == 0 {
			break
		}
		offset += len(payload.Metas)
	}
	return chart, nil
}

func wikidataChart(ctx context.Context, kind string, year int, genre string) (annualChart, error) {
	chart := annualChart{Year: year, Source: "Wikidata · verknüpfte Artikel", Items: []entry{}}
	classes, identifier := "wd:Q11424", "?item wdt:P345 ?catalog ."
	dateProperties := "wdt:P577"
	if kind == "series" {
		classes = "wd:Q5398426 wd:Q1259759"
		dateProperties = "wdt:P577 wdt:P580"
	}
	if kind == "game" {
		classes = "wd:Q7889"
		identifier = "OPTIONAL { ?item wdt:P1733 ?steam . }"
	}
	filter := ""
	if criterion := chartGenres[kind][genre]; genre != "" {
		if criterion.wikidata == "" {
			return chart, fmt.Errorf("Genre bei Wikidata nicht verfügbar")
		}
		filter = "VALUES ?genre { " + criterion.wikidata + " } ?item wdt:P136 ?genre ."
	}
	query := fmt.Sprintf(`SELECT DISTINCT ?item ?catalog ?steam ?title ?links WHERE {
      VALUES ?class { %s }
      VALUES ?dateProperty { %s }
      ?item wdt:P31 ?class; ?dateProperty ?date; wikibase:sitelinks ?links .
      FILTER("%d-01-01T00:00:00Z"^^xsd:dateTime <= ?date && ?date <= "%sT23:59:59Z"^^xsd:dateTime)
      FILTER NOT EXISTS { ?item ?dateProperty ?earlier . FILTER(?earlier < "%d-01-01T00:00:00Z"^^xsd:dateTime) }
      %s %s
      ?item rdfs:label ?title FILTER(LANG(?title)="en")
    } ORDER BY DESC(?links) LIMIT 80`, classes, dateProperties, year, time.Now().Format("2006-01-02"), year, identifier, filter)
	params := url.Values{"query": {query}, "format": {"json"}}
	var payload struct {
		Results struct {
			Bindings []struct{ Item, Catalog, Steam, Title struct{ Value string } }
		}
	}
	err := getJSON(ctx, "https://query.wikidata.org/sparql?"+params.Encode(), "", &payload)
	seen := map[string]bool{}
	for _, value := range payload.Results.Bindings {
		id, title, cover := value.Catalog.Value, value.Title.Value, ""
		if kind == "game" {
			qid := strings.TrimPrefix(strings.TrimPrefix(value.Item.Value, "http://www.wikidata.org/entity/"), "https://www.wikidata.org/entity/")
			if !wikidataID.MatchString(qid) {
				continue
			}
			id = "game:wikidata:" + qid
			if steam := value.Steam.Value; numericID.MatchString(steam) {
				id = "game:steam:" + steam
				cover = "https://cdn.cloudflare.steamstatic.com/steam/apps/" + steam + "/header.jpg"
			}
		} else {
			if !imdbID.MatchString(id) {
				continue
			}
			cover = "https://images.metahub.space/poster/small/" + id + "/img"
			id = kind + ":imdb:" + id
		}
		if title == "" || seen[id] {
			continue
		}
		seen[id] = true
		var genres []string
		if genre != "" {
			genres = []string{genre}
		}
		chart.Items = append(chart.Items, entry{ID: id, Kind: kind, Title: title, Subtitle: strconv.Itoa(year), Cover: cover, Source: "Wikidata", Genres: genres})
		if len(chart.Items) == 50 {
			break
		}
	}
	return chart, err
}

func imdbChart(ctx context.Context, kind string, year int, genre string) (annualChart, error) {
	chart := annualChart{Year: year, Source: "IMDbAPI · Beliebtheit", Items: []entry{}}
	yearText := strconv.Itoa(year)
	params := url.Values{"startYear": {yearText}, "endYear": {yearText}, "limit": {"50"}, "sortBy": {"SORT_BY_POPULARITY"}, "sortOrder": {"ASC"}}
	if kind == "series" {
		params["types"] = []string{"TV_SERIES", "TV_MINI_SERIES"}
	} else {
		params.Set("types", "MOVIE")
	}
	if genre != "" {
		params.Set("genres", chartGenres[kind][genre].term)
	}
	var payload struct {
		Titles []struct {
			ID, PrimaryTitle, OriginalTitle, Plot string
			StartYear                             int
			IsAdult                               bool
			Genres                                []string
			PrimaryImage                          struct{ URL string }
		}
	}
	err := getJSON(ctx, "https://api.imdbapi.dev/titles?"+params.Encode(), "", &payload)
	seen := map[string]bool{}
	for _, title := range payload.Titles {
		if title.IsAdult || title.StartYear != year || title.PrimaryTitle == "" || !imdbID.MatchString(title.ID) || seen[title.ID] {
			continue
		}
		seen[title.ID] = true
		chart.Items = append(chart.Items, entry{ID: kind + ":imdb:" + title.ID, Kind: kind, Title: title.PrimaryTitle, Subtitle: yearText,
			Cover: title.PrimaryImage.URL, Description: limitDescription(title.Plot), Genres: prettyGenres(title.Genres), Source: "IMDbAPI", Aliases: mergeLabels(nil, []string{title.OriginalTitle})})
		if len(chart.Items) == 50 {
			break
		}
	}
	return chart, err
}

func openLibraryDetail(ctx context.Context, key string) (entry, error) {
	var payload struct {
		Subjects    []string
		Description json.RawMessage
	}
	err := getJSON(ctx, "https://openlibrary.org"+key+".json", "", &payload)
	var description string
	if json.Unmarshal(payload.Description, &description) != nil {
		var rich struct{ Value string }
		if json.Unmarshal(payload.Description, &rich) == nil {
			description = rich.Value
		}
	}
	return entry{Genres: classifyBookGenres(payload.Subjects), Description: limitDescription(description)}, err
}

func limitDescription(value string) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) > 1500 {
		return string(runes[:1500]) + " …"
	}
	return string(runes)
}
