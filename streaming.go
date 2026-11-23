package main

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type offer struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Price    *float64 `json:"price,omitempty"`
	Currency string   `json:"currency,omitempty"`
	URL      string   `json:"url,omitempty"`
}

type availability struct {
	Offers    []offer `json:"offers"`
	Link      string  `json:"link,omitempty"`
	TMDB      bool    `json:"tmdb"`
	Watchmode bool    `json:"watchmode"`
	Partial   bool    `json:"partial"`
	Region    string  `json:"region"`
}

type cachedOffers struct {
	value   availability
	expires time.Time
}

var offerCache sync.Map

func fromTMDB(ctx context.Context, kind, source, key string, prefs settings) (availability, error) {
	method := tmdbKind(kind)
	if source == "imdb" {
		var matches struct {
			Movies []tmdbTitle `json:"movie_results"`
			Series []tmdbTitle `json:"tv_results"`
		}
		err := getJSON(ctx, "https://api.themoviedb.org/3/find/"+key+"?external_source=imdb_id", prefs.TMDB, &matches)
		if err != nil {
			return availability{}, err
		}
		titles := matches.Movies
		if kind == "series" {
			titles = matches.Series
		}
		if len(titles) == 0 {
			return availability{}, nil
		}
		key = strings.Split(titles[0].internal(kind).ID, ":")[2]
	}
	type provider struct {
		Name string `json:"provider_name"`
	}
	var payload struct {
		Results map[string]struct {
			Link                           string
			Flatrate, Free, Ads, Rent, Buy []provider
		}
	}
	err := getJSON(ctx, "https://api.themoviedb.org/3/"+method+"/"+key+"/watch/providers", prefs.TMDB, &payload)
	if err != nil {
		return availability{}, err
	}
	result := availability{TMDB: true}
	local := payload.Results[prefs.Region]
	result.Link = local.Link
	for _, group := range []struct {
		label     string
		providers []provider
	}{
		{"sub", local.Flatrate}, {"free", local.Free}, {"ads", local.Ads}, {"rent", local.Rent}, {"buy", local.Buy},
	} {
		for _, row := range group.providers {
			if row.Name != "" {
				result.Offers = append(result.Offers, offer{Name: row.Name, Type: group.label})
			}
		}
	}
	return result, nil
}

func fromWatchmode(ctx context.Context, kind, source, key string, prefs settings) ([]offer, error) {
	if source == "tmdb" {
		key = tmdbKind(kind) + "-" + key
	}
	q := url.Values{"apiKey": {prefs.Watchmode}, "regions": {prefs.Region}}
	var payload []struct {
		Name, Type, Region, Currency string
		Price                        *float64
		URL                          string `json:"web_url"`
	}
	err := getJSON(ctx, "https://api.watchmode.com/v1/title/"+key+"/sources/?"+q.Encode(), "", &payload)
	if err != nil {
		return nil, err
	}
	currencies := map[string]string{"DE": "EUR", "AT": "EUR", "CH": "CHF", "US": "USD", "GB": "GBP", "CA": "CAD", "AU": "AUD", "FR": "EUR", "IT": "EUR", "ES": "EUR", "NL": "EUR"}
	result := []offer{}
	for _, row := range payload {
		if row.Region != prefs.Region || row.Name == "" {
			continue
		}
		switch row.Type {
		case "sub", "rent", "buy", "free", "ads", "tve":
		default:
			continue
		}
		currency := strings.ToUpper(row.Currency)
		if currency == "" {
			currency = currencies[prefs.Region]
		}
		price := row.Price
		if (row.Type != "rent" && row.Type != "buy") || (price != nil && *price <= 0) {
			price = nil
		}
		result = append(result, offer{Name: row.Name, Type: row.Type, Price: price, Currency: currency, URL: row.URL})
	}
	return result, nil
}

func mergedOffers(primary, fallback []offer) []offer {
	items := []offer{}
	seen := map[string]int{}
	for _, candidate := range append(primary, fallback...) {
		key := strings.ToLower(candidate.Name) + "|" + candidate.Type
		if i, exists := seen[key]; exists {
			prior := items[i]
			if candidate.Price != nil && (prior.Price == nil || *candidate.Price < *prior.Price) {
				items[i] = candidate
			}
			continue
		}
		seen[key] = len(items)
		items = append(items, candidate)
	}
	return items
}

func (a *app) streaming(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	parts := strings.Split(id, ":")
	prefs := a.getSettings()
	if len(parts) != 3 || (parts[0] != "movie" && parts[0] != "series") || !((parts[1] == "imdb" && imdbID.MatchString(parts[2])) || (parts[1] == "tmdb" && numericID.MatchString(parts[2]))) {
		fail(w, 400, "Für diesen Eintrag fehlen Katalogdaten.")
		return
	}
	if prefs.TMDB == "" && prefs.Watchmode == "" {
		fail(w, 422, "Aktiviere die Streaming-Suche in den Einstellungen.")
		return
	}
	cacheKey := struct {
		id, tmdb, watchmode, region string
	}{id, prefs.TMDB, prefs.Watchmode, prefs.Region}
	if cached, ok := offerCache.Load(cacheKey); ok {
		value := cached.(cachedOffers)
		if time.Now().Before(value.expires) {
			reply(w, 200, value.value)
			return
		}
	}
	result := availability{Region: prefs.Region}
	var tmdbErr, watchErr error
	var primary, fallback []offer
	var wg sync.WaitGroup
	if prefs.Watchmode != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			primary, watchErr = fromWatchmode(r.Context(), parts[0], parts[1], parts[2], prefs)
		}()
	}
	if prefs.TMDB != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var tmdb availability
			tmdb, tmdbErr = fromTMDB(r.Context(), parts[0], parts[1], parts[2], prefs)
			result.Link, result.TMDB = tmdb.Link, tmdb.TMDB
			fallback = tmdb.Offers
		}()
	}
	wg.Wait()
	if (prefs.Watchmode == "" || watchErr != nil) && (prefs.TMDB == "" || tmdbErr != nil) {
		message := "Streamingdaten sind gerade nicht verfügbar."
		if errors.Is(r.Context().Err(), context.DeadlineExceeded) || strings.Contains(fmtErrors(watchErr, tmdbErr), "HTTP 401") {
			message = "Streamingzugang nicht verfügbar. Prüfe deine API-Daten in den Einstellungen."
		}
		fail(w, 502, message)
		return
	}
	result.Watchmode = prefs.Watchmode != "" && watchErr == nil
	result.Partial = watchErr != nil || tmdbErr != nil
	result.Offers = mergedOffers(primary, fallback)
	ttl := 6 * time.Hour
	if result.Partial {
		ttl = 10 * time.Minute
	}
	offerCache.Store(cacheKey, cachedOffers{result, time.Now().Add(ttl)})
	reply(w, 200, result)
}

func fmtErrors(errs ...error) string {
	var all string
	for _, err := range errs {
		if err != nil {
			all += " " + err.Error()
		}
	}
	return all
}
