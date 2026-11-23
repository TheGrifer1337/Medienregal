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
	year := time.Now().Year()
	token := a.getSettings().TMDB
	key := struct {
		kind  string
		year  int
		token string
	}{kind, year, token}
	if val, ok := annualCharts.Load(key); ok {
		cached := val.(chartCache)
		if time.Now().Before(cached.Until) {
			reply(w, 200, cached.Chart)
			return
		}
	}
	chart, err := loadChart(r.Context(), kind, year, token)
	if err != nil || len(chart.Items) == 0 {
		fail(w, 502, "Die Jahresliste ist gerade nicht verfügbar.")
		return
	}
	annualCharts.Store(key, chartCache{Chart: chart, Until: time.Now().Add(40 * time.Minute)})
	reply(w, 200, chart)
}

func loadChart(ctx context.Context, kind string, year int, token string) (annualChart, error) {
	if kind == "book" {
		return bookChart(ctx, year)
	}
	if token != "" {
		if chart, err := tmdbChart(ctx, kind, year, token); err == nil && len(chart.Items) > 0 {
			return chart, nil
		}
	}
	result, err := cinemetaChart(ctx, kind, year)
	if err == nil && len(result.Items) == 50 {
		return result, nil
	}
	fallback, fallbackErr := wikidataChart(ctx, kind, year)
	if fallbackErr == nil && len(fallback.Items) > len(result.Items) {
		return fallback, nil
	}
	if len(result.Items) == 0 {
		if fallback, fallbackErr = imdbChart(ctx, kind, year); fallbackErr == nil && len(fallback.Items) > 0 {
			return fallback, nil
		}
	}
	return result, err
}

func bookChart(ctx context.Context, year int) (annualChart, error) {
	params := "q=first_publish_year%3A" + strconv.Itoa(year) + "&sort=readinglog&limit=50&fields=key,title,author_name,cover_i,readinglog_count,first_publish_year"
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

func tmdbChart(ctx context.Context, kind string, year int, token string) (annualChart, error) {
	chart := annualChart{Year: year, Source: "TMDB · Beliebtheit", Items: []entry{}}
	q := url.Values{"language": {"de-DE"}, "sort_by": {"popularity.desc"}, "include_adult": {"false"}}
	if kind == "movie" {
		q.Set("primary_release_year", strconv.Itoa(year))
		q.Set("primary_release_date.lte", time.Now().Format("2006-01-02"))
	} else {
		q.Set("first_air_date_year", strconv.Itoa(year))
		q.Set("first_air_date.lte", time.Now().Format("2006-01-02"))
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

func cinemetaChart(ctx context.Context, kind string, year int) (annualChart, error) {
	chart := annualChart{Year: year, Source: "Cinemeta · Jahreskatalog", Items: []entry{}}
	seen := map[string]bool{}
	for offset := 0; offset < 100 && len(chart.Items) < 50; {
		var payload struct{ Metas []cinemeta }
		filter := fmt.Sprintf("genre=%d", year)
		if offset > 0 {
			filter += fmt.Sprintf("&skip=%d", offset)
		}
		if err := getJSON(ctx, "https://v3-cinemeta.strem.io/catalog/"+kind+"/year/"+filter+".json", "", &payload); err != nil {
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
			chart.Items = append(chart.Items, title.internal(kind))
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

func wikidataChart(ctx context.Context, kind string, year int) (annualChart, error) {
	chart := annualChart{Year: year, Source: "Wikidata · verknüpfte Artikel", Items: []entry{}}
	classes := "wd:Q11424"
	if kind == "series" {
		classes = "wd:Q5398426 wd:Q1259759"
	}
	query := fmt.Sprintf(`SELECT DISTINCT ?item ?imdb ?title ?links WHERE {
      VALUES ?class { %s }
      ?item wdt:P31 ?class; wdt:P577 ?date; wdt:P345 ?imdb; wikibase:sitelinks ?links .
      FILTER("%d-01-01T00:00:00Z"^^xsd:dateTime <= ?date && ?date <= "%sT23:59:59Z"^^xsd:dateTime)
      ?item rdfs:label ?title FILTER(LANG(?title)="en")
    } ORDER BY DESC(?links) LIMIT 70`, classes, year, time.Now().Format("2006-01-02"))
	params := url.Values{"query": {query}, "format": {"json"}}
	var payload struct {
		Results struct {
			Bindings []struct {
				IMDb, Title struct{ Value string }
			}
		}
	}
	err := getJSON(ctx, "https://query.wikidata.org/sparql?"+params.Encode(), "", &payload)
	seen := map[string]bool{}
	for _, value := range payload.Results.Bindings {
		id, title := value.IMDb.Value, value.Title.Value
		if !imdbID.MatchString(id) || title == "" || seen[id] {
			continue
		}
		seen[id] = true
		chart.Items = append(chart.Items, entry{ID: kind + ":imdb:" + id, Kind: kind, Title: title, Subtitle: strconv.Itoa(year),
			Cover: "https://images.metahub.space/poster/small/" + id + "/img", Source: "Wikidata"})
		if len(chart.Items) == 50 {
			break
		}
	}
	return chart, err
}

func imdbChart(ctx context.Context, kind string, year int) (annualChart, error) {
	chart := annualChart{Year: year, Source: "IMDbAPI · Beliebtheit", Items: []entry{}}
	yearText := strconv.Itoa(year)
	params := url.Values{"startYear": {yearText}, "endYear": {yearText}, "limit": {"50"}, "sortBy": {"SORT_BY_POPULARITY"}, "sortOrder": {"ASC"}}
	if kind == "series" {
		params["types"] = []string{"TV_SERIES", "TV_MINI_SERIES"}
	} else {
		params.Set("types", "MOVIE")
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
