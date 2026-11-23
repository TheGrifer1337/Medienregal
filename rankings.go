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
	Region string  `json:"region,omitempty"`
	Scope  string  `json:"scope,omitempty"`
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
	prefs := a.getSettings()
	region := prefs.Region
	if region == "" {
		region = "DE"
	}
	key := struct {
		kind, genre, region, language string
		year                          int
		token                         string
	}{kind, genre, region, prefs.locale(), year, prefs.TMDB}
	if val, ok := annualCharts.Load(key); ok {
		cached := val.(chartCache)
		if time.Now().Before(cached.Until) {
			reply(w, 200, cached.Chart)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 22*time.Second)
	defer cancel()
	chart, err := regionalChart(ctx, kind, year, prefs, genre, region)
	if err != nil {
		fail(w, 502, "Die Jahresliste ist gerade nicht verfügbar.")
		return
	}
	annualCharts.Store(key, chartCache{Chart: chart, Until: time.Now().Add(40 * time.Minute)})
	reply(w, 200, chart)
}

func regionalChart(ctx context.Context, kind string, year int, prefs settings, genre, region string) (annualChart, error) {
	if kind == "book" {
		return bookChart(ctx, year, genre, region)
	}
	if kind == "game" {
		return steamChart(ctx, year, genre, region)
	}
	if prefs.TMDB != "" && (genre == "" || chartGenres[kind][genre].tmdb != "") {
		tmdbContext, cancel := context.WithTimeout(ctx, 7*time.Second)
		chart, err := tmdbChart(tmdbContext, kind, year, prefs.TMDB, genre, region, prefs.tmdbLocale())
		cancel()
		if len(chart.Items) > 0 || (err == nil && ctx.Err() != nil) {
			return chart, nil
		}
	}
	apiContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	result, err := imdbChart(apiContext, kind, year, genre, region)
	cancel()
	if len(result.Items) > 0 {
		return result, nil
	}
	fallback, fallbackErr := cinemetaChart(ctx, kind, year, genre, region)
	if len(fallback.Items) > 0 || fallbackErr == nil {
		return fallback, nil
	}
	if err == nil {
		return result, nil
	}
	return fallback, fallbackErr
}

var marketBookLanguage = map[string]string{
	"DE": "ger", "AT": "ger", "CH": "ger", "US": "eng", "GB": "eng", "CA": "eng", "AU": "eng",
	"CN": "chi", "TW": "chi", "HK": "chi", "JP": "jpn", "KR": "kor", "FR": "fre", "ES": "spa",
	"MX": "spa", "IT": "ita", "BR": "por", "PT": "por", "NL": "dut", "PL": "pol", "SE": "swe",
}

var marketNames = map[string][]string{
	"DE": {"Germany", "West Germany", "East Germany"}, "AT": {"Austria"}, "CH": {"Switzerland"},
	"US": {"USA", "United States", "United States of America"}, "GB": {"UK", "United Kingdom"},
	"CA": {"Canada"}, "AU": {"Australia"}, "CN": {"China", "Mainland China"}, "TW": {"Taiwan"},
	"HK": {"Hong Kong"}, "JP": {"Japan"}, "KR": {"South Korea", "Korea, South"}, "FR": {"France"},
	"ES": {"Spain"}, "MX": {"Mexico"}, "IT": {"Italy"}, "BR": {"Brazil"}, "PT": {"Portugal"},
	"NL": {"Netherlands", "The Netherlands"}, "PL": {"Poland"}, "SE": {"Sweden"},
}

func chartCountry(country, region string) bool {
	for _, code := range strings.Split(country, ",") {
		if strings.EqualFold(strings.TrimSpace(code), region) {
			return true
		}
	}
	for _, name := range marketNames[region] {
		if strings.Contains(","+strings.ReplaceAll(strings.ToLower(country), ", ", ",")+",", ","+strings.ToLower(name)+",") {
			return true
		}
	}
	return false
}

func bookChart(ctx context.Context, year int, genre, region string) (annualChart, error) {
	query := "first_publish_year:" + strconv.Itoa(year)
	if genre != "" {
		query += " AND subject:" + strconv.Quote(chartGenres["book"][genre].term)
	}
	language := marketBookLanguage[region]
	if language != "" {
		query += " AND language:" + language
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
	chart := annualChart{Year: year, Source: "Open Library", Region: region, Scope: "book-language", Items: []entry{}}
	if language == "" {
		chart.Scope = "book-global"
	}
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

func tmdbChart(ctx context.Context, kind string, year int, token, genre, region, language string) (annualChart, error) {
	chart := annualChart{Year: year, Source: "TMDB", Region: region, Scope: "streaming-market", Items: []entry{}}
	q := url.Values{"language": {language}, "sort_by": {"popularity.desc"}, "include_adult": {"false"}, "watch_region": {region}, "with_watch_monetization_types": {"flatrate|free|ads|rent|buy"}}
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

func cinemetaChart(ctx context.Context, kind string, year int, genre, region string) (annualChart, error) {
	chart := annualChart{Year: year, Source: "Cinemeta", Region: region, Scope: "production-country", Items: []entry{}}
	catalog, maxOffset := "year", 400
	yearLabel := strconv.Itoa(year)
	if genre != "" {
		catalog, maxOffset = "top", 400
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
			if !chartCountry(title.Country, region) || (genre != "" && !strings.HasPrefix(title.ReleaseInfo, yearLabel)) {
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

func imdbChart(ctx context.Context, kind string, year int, genre, region string) (annualChart, error) {
	chart := annualChart{Year: year, Source: "IMDbAPI", Region: region, Scope: "production-country", Items: []entry{}}
	yearText := strconv.Itoa(year)
	params := url.Values{"startYear": {yearText}, "endYear": {yearText}, "limit": {"50"}, "sortBy": {"SORT_BY_POPULARITY"}, "sortOrder": {"ASC"}, "countryCodes": {region}}
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
			Rating                                struct{ AggregateRating float64 }
			OriginCountries                       []struct{ Code string }
		}
	}
	err := getJSON(ctx, "https://api.imdbapi.dev/titles?"+params.Encode(), "", &payload)
	seen := map[string]bool{}
	for _, title := range payload.Titles {
		if title.IsAdult || title.StartYear != year || title.PrimaryTitle == "" || !imdbID.MatchString(title.ID) || seen[title.ID] {
			continue
		}
		if len(title.OriginCountries) > 0 {
			matched := false
			for _, country := range title.OriginCountries {
				if country.Code == region {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		score := 0.0
		if kind == "movie" && title.Rating.AggregateRating >= 1 && title.Rating.AggregateRating <= 10 {
			score = title.Rating.AggregateRating
		}
		seen[title.ID] = true
		chart.Items = append(chart.Items, entry{IMDb: score, ID: kind + ":imdb:" + title.ID, Kind: kind, Title: title.PrimaryTitle, Subtitle: yearText,
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
