package main

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type mediaLoan struct {
	To    string `json:"to"`
	Since string `json:"since,omitempty"`
	Due   string `json:"due,omitempty"`
}

func validMediaExtras(item entry) bool {
	if item.Trailer != "" && (item.Kind != "movie" || !youtubeID.MatchString(item.Trailer)) {
		return false
	}
	if len(item.Series) > 360 || item.Sequence < 0 || item.Sequence > 100000 || item.Sequence != 0 && strings.TrimSpace(item.Series) == "" || math.IsNaN(item.Sequence) ||
		item.IMDb != 0 && (item.Kind != "movie" || item.IMDb < 1 || item.IMDb > 10 || math.IsNaN(item.IMDb)) {
		return false
	}
	if item.Loan == nil {
		return true
	}
	if strings.TrimSpace(item.Loan.To) == "" || len(item.Loan.To) > 360 {
		return false
	}
	for _, date := range []string{item.Loan.Since, item.Loan.Due} {
		if date != "" {
			if _, err := time.Parse(time.DateOnly, date); err != nil {
				return false
			}
		}
	}
	return item.Loan.Due == "" || item.Loan.Since == "" || item.Loan.Due >= item.Loan.Since
}

func (s settings) locale() string {
	switch s.Language {
	case "en", "zh-Hans":
		return s.Language
	default:
		return "de"
	}
}

func (s settings) languageCode() string {
	return strings.SplitN(s.locale(), "-", 2)[0]
}

func (s settings) tmdbLocale() string {
	return map[string]string{"de": "de-DE", "en": "en-US", "zh-Hans": "zh-CN"}[s.locale()]
}

func (s settings) steamLocale() string {
	return map[string]string{"de": "german", "en": "english", "zh-Hans": "schinese"}[s.locale()]
}

func (a *app) goals(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if r.Method == http.MethodGet {
		goals := a.data.Goals
		if goals == nil {
			goals = map[int]map[string]int{}
		}
		reply(w, 200, goals)
		return
	}
	var request struct {
		Year    int
		Targets map[string]int
	}
	if !decode(w, r, &request) {
		return
	}
	if request.Year < 1900 || request.Year > 2200 || len(request.Targets) == 0 || len(request.Targets) > len(statuses) {
		fail(w, 400, "Das Jahresziel ist ungültig.")
		return
	}
	for kind, goal := range request.Targets {
		if _, ok := statuses[kind]; !ok || goal < 0 || goal > 10000 {
			fail(w, 400, "Das Jahresziel ist ungültig.")
			return
		}
	}
	next := a.data
	next.Goals = make(map[int]map[string]int, len(a.data.Goals)+1)
	for year, existing := range a.data.Goals {
		next.Goals[year] = existing
	}
	selected := map[string]int{}
	for kind, goal := range a.data.Goals[request.Year] {
		selected[kind] = goal
	}
	for kind, goal := range request.Targets {
		if goal == 0 {
			delete(selected, kind)
		} else {
			selected[kind] = goal
		}
	}
	if len(selected) == 0 {
		delete(next.Goals, request.Year)
	} else {
		next.Goals[request.Year] = selected
	}
	if err := writeLibrary(a.path, next); err != nil {
		fail(w, 500, "Das Jahresziel konnte nicht gespeichert werden.")
		return
	}
	a.data = next
	reply(w, 200, next.Goals)
}

type imdbScore struct {
	Score float64 `json:"score"`
	ID    string  `json:"id,omitempty"`
}

type imdbAPIData struct {
	ID     string
	Rating struct{ AggregateRating float64 }
}

type imdbScoreCache struct {
	Value imdbScore
	Until time.Time
}

var imdbScores sync.Map

func parseIMDb(raw json.RawMessage) float64 {
	value, _ := strconv.ParseFloat(strings.Trim(string(raw), `"`), 64)
	if value >= 1 && value <= 10 {
		return value
	}
	return 0
}

func (a *app) imdbRating(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	id, title, year := params.Get("id"), strings.TrimSpace(params.Get("title")), params.Get("year")
	if len(r.URL.RawQuery) > 1700 || len(title) > 600 {
		fail(w, 400, "Ungültiger Filmtitel.")
		return
	}
	prefs := a.getSettings()
	key := strings.Join([]string{id, coverName(title), year, prefs.TMDB}, "\x00")
	if value, ok := imdbScores.Load(key); ok {
		cached := value.(imdbScoreCache)
		if time.Now().Before(cached.Until) {
			reply(w, 200, cached.Value)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 9*time.Second)
	defer cancel()
	score := findIMDbRating(ctx, prefs, id, title, year)
	ttl := 10 * time.Minute
	if score.Score > 0 {
		ttl = 8 * time.Hour
	}
	imdbScores.Store(key, imdbScoreCache{score, time.Now().Add(ttl)})
	reply(w, 200, score)
}

func identifyMovie(ctx context.Context, prefs settings, id, title, year string) imdbScore {
	var score imdbScore
	if key, ok := strings.CutPrefix(id, "movie:imdb:"); ok && imdbID.MatchString(key) {
		score.ID = key
	} else if key, ok := strings.CutPrefix(id, "movie:tmdb:"); ok && numericID.MatchString(key) && prefs.TMDB != "" {
		var result struct {
			ID string `json:"imdb_id"`
		}
		_ = getJSON(ctx, "https://api.themoviedb.org/3/movie/"+key+"/external_ids", prefs.TMDB, &result)
		if imdbID.MatchString(result.ID) {
			score.ID = result.ID
		}
	}
	if score.ID == "" && title != "" {
		items, _ := searchCinemeta(ctx, "movie", title)
		query := coverQuery{Kind: "movie", Title: title, Subtitle: year}
		for _, item := range items {
			if !query.matchesTitle(item.Title, item.Aliases...) || !query.matchesYear(item.Subtitle) {
				continue
			}
			id := strings.TrimPrefix(item.ID, "movie:imdb:")
			if score.ID != "" && score.ID != id {
				return imdbScore{}
			}
			score = imdbScore{item.IMDb, id}
		}
	}
	return score
}

func findIMDbRating(ctx context.Context, prefs settings, id, title, year string) imdbScore {
	score := identifyMovie(ctx, prefs, id, title, year)
	if score.ID == "" || score.Score > 0 {
		return score
	}
	var detail struct{ Meta cinemeta }
	_ = getJSON(ctx, "https://v3-cinemeta.strem.io/meta/movie/"+score.ID+".json", "", &detail)
	score.Score = parseIMDb(detail.Meta.IMDbRating)
	if score.Score == 0 {
		var result imdbAPIData
		if getJSON(ctx, "https://api.imdbapi.dev/titles/"+score.ID, "", &result) == nil && result.ID == score.ID && result.Rating.AggregateRating >= 1 && result.Rating.AggregateRating <= 10 {
			score.Score = result.Rating.AggregateRating
		}
	}
	return score
}

func (a *app) launchGame(w http.ResponseWriter, r *http.Request) {
	var request struct{ ID, Title string }
	if !decode(w, r, &request) {
		return
	}
	if len(request.ID) > 200 || len(request.Title) > 600 {
		fail(w, 400, "Ungültiges Spiel.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	paths := localLauncherLocations(ctx)
	installed := scanLaunchers(ctx, paths)
	id := request.ID
	if strings.HasPrefix(id, "manual:") || strings.HasPrefix(id, "game:wikidata:") {
		id = ""
		for _, item := range installed {
			if historyKey(item.Title) == historyKey(request.Title) {
				if id != "" {
					fail(w, 409, "Mehrere lokale Installationen passen zu diesem Spiel.")
					return
				}
				id = item.ID
			}
		}
	}
	for _, item := range installed {
		if item.ID != id {
			continue
		}
		if err := openInstalledGame(ctx, item); err != nil {
			fail(w, 409, "Für dieses Spiel ist kein nutzbarer Startlink verfügbar.")
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	fail(w, 404, "Dieses Spiel wurde in den installierten Launchern nicht gefunden.")
}

func epicLaunchURL(appName, namespace, catalog string) string {
	key := url.PathEscape(appName)
	if namespace != "" && catalog != "" {
		key = strings.Join([]string{url.PathEscape(namespace), url.PathEscape(catalog), key}, "%3A")
	}
	return "com.epicgames.launcher://apps/" + key + "?action=launch&silent=true"
}

func (a *app) movieTrailer(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	if len(r.URL.RawQuery) > 1700 {
		fail(w, 400, "Ungültiger Filmtitel.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 9*time.Second)
	defer cancel()
	id := params.Get("id")
	prefs := a.getSettings()
	trailer := ""
	if key, ok := strings.CutPrefix(id, "movie:tmdb:"); ok && numericID.MatchString(key) && prefs.TMDB != "" {
		var data tmdbTitle
		values := url.Values{"language": {prefs.tmdbLocale()}, "append_to_response": {"videos"}, "include_video_language": {prefs.languageCode() + ",en"}}
		if getJSON(ctx, "https://api.themoviedb.org/3/movie/"+key+"?"+values.Encode(), prefs.TMDB, &data) == nil {
			trailer = data.youtubeTrailer(prefs.languageCode())
		}
	}
	if trailer == "" {
		movie := identifyMovie(ctx, prefs, id, params.Get("title"), params.Get("year"))
		if movie.ID != "" {
			var data struct{ Meta cinemeta }
			if getJSON(ctx, "https://v3-cinemeta.strem.io/meta/movie/"+movie.ID+".json", "", &data) == nil {
				trailer = data.Meta.youtubeTrailer()
			}
		}
	}
	reply(w, 200, map[string]string{"trailer": trailer})
}

func (a *app) movieSeries(w http.ResponseWriter, r *http.Request) {
	prefs := a.getSettings()
	id := r.URL.Query().Get("id")
	result := struct {
		Name  string  `json:"name"`
		Items []entry `json:"items"`
	}{Items: []entry{}}
	if prefs.TMDB == "" || len(id) > 200 {
		reply(w, 200, result)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 9*time.Second)
	defer cancel()
	tmdbID, _ := strings.CutPrefix(id, "movie:tmdb:")
	if imdb, ok := strings.CutPrefix(id, "movie:imdb:"); ok && imdbID.MatchString(imdb) {
		var matches struct {
			Movies []struct{ ID int } `json:"movie_results"`
		}
		if getJSON(ctx, "https://api.themoviedb.org/3/find/"+imdb+"?external_source=imdb_id", prefs.TMDB, &matches) == nil && len(matches.Movies) == 1 {
			tmdbID = strconv.Itoa(matches.Movies[0].ID)
		}
	}
	if !numericID.MatchString(tmdbID) {
		reply(w, 200, result)
		return
	}
	suffix := "?" + url.Values{"language": {prefs.tmdbLocale()}}.Encode()
	var film struct {
		Collection struct{ ID int } `json:"belongs_to_collection"`
	}
	if getJSON(ctx, "https://api.themoviedb.org/3/movie/"+tmdbID+suffix, prefs.TMDB, &film) != nil || film.Collection.ID <= 0 {
		reply(w, 200, result)
		return
	}
	var collection struct {
		Name  string
		Parts []tmdbTitle
	}
	if getJSON(ctx, "https://api.themoviedb.org/3/collection/"+strconv.Itoa(film.Collection.ID)+suffix, prefs.TMDB, &collection) != nil {
		fail(w, 502, "Die Filmreihe konnte gerade nicht geladen werden.")
		return
	}
	sort.SliceStable(collection.Parts, func(i, j int) bool {
		left, right := collection.Parts[i], collection.Parts[j]
		if left.Release == "" || right.Release == "" {
			return left.Release != "" && right.Release == ""
		}
		return left.Release < right.Release
	})
	result.Name = collection.Name
	for _, part := range collection.Parts {
		if part.ID > 0 && part.Title != "" {
			result.Items = append(result.Items, part.internal("movie"))
		}
	}
	reply(w, 200, result)
}
