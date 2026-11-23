package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

var steamUserID = regexp.MustCompile(`^[0-9]{17}$`)
var steamAPIKey = regexp.MustCompile(`^[a-fA-F0-9]{32}$`)

type steamActivity struct {
	AppID      int   `json:"appid"`
	Minutes    int   `json:"playtime_forever"`
	LastPlayed int64 `json:"rtime_last_played"`
}

type steamActivityCache struct {
	Games map[string]steamActivity
	Until time.Time
}

var steamActivities sync.Map

func (a *app) steamInfo(w http.ResponseWriter, r *http.Request) {
	prefs := a.getSettings()
	if !steamUserID.MatchString(prefs.SteamUser) || !steamAPIKey.MatchString(prefs.SteamKey) {
		reply(w, 200, map[string]steamActivity{})
		return
	}
	cacheKey := sha256.Sum256([]byte(prefs.SteamUser + prefs.SteamKey))
	if value, ok := steamActivities.Load(cacheKey); ok {
		snapshot := value.(steamActivityCache)
		if time.Now().Before(snapshot.Until) {
			reply(w, 200, snapshot.Games)
			return
		}
	}
	var data struct {
		Response struct {
			Games []steamActivity
		}
	}
	input, _ := json.Marshal(map[string]any{"steamid": prefs.SteamUser, "include_played_free_games": true})
	params := url.Values{"key": {prefs.SteamKey}, "input_json": {string(input)}}
	if getJSON(r.Context(), "https://api.steampowered.com/IPlayerService/GetOwnedGames/v1/?"+params.Encode(), "", &data) != nil {
		fail(w, 502, "Steam-Spielzeit ist gerade nicht verfügbar.")
		return
	}
	games := map[string]steamActivity{}
	for _, game := range data.Response.Games {
		if game.AppID > 0 && game.Minutes >= 0 && game.LastPlayed >= 0 {
			games[strconv.Itoa(game.AppID)] = game
		}
	}
	steamActivities.Store(cacheKey, steamActivityCache{games, time.Now().Add(10 * time.Minute)})
	reply(w, 200, games)
}

type titleAdvisory struct {
	Ratings   []string `json:"ratings,omitempty"`
	Note      string   `json:"note,omitempty"`
	Warnings  []string `json:"warnings,omitempty"`
	Source    string   `json:"source,omitempty"`
	Reference string   `json:"reference,omitempty"`
}

func (a *app) advisories(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	if len(r.URL.RawQuery) > 1700 {
		fail(w, 400, "Ungültiger Katalogeintrag.")
		return
	}
	prefs := a.getSettings()
	pieces := strings.Split(params.Get("id"), ":")
	if len(pieces) != 3 {
		reply(w, 200, titleAdvisory{})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	kind, source, id := pieces[0], pieces[1], pieces[2]
	result := titleAdvisory{}
	switch {
	case kind == "game" && source == "steam" && numericID.MatchString(id):
		values := url.Values{"appids": {id}, "cc": {prefs.Region}, "l": {prefs.steamLocale()}}
		var payload map[string]struct {
			Success bool
			Data    struct {
				Boards      map[string]struct{ Rating string } `json:"ratings"`
				Age         json.RawMessage                    `json:"required_age"`
				Descriptors struct{ Notes string }             `json:"content_descriptors"`
			}
		}
		_ = getJSON(ctx, "https://store.steampowered.com/api/appdetails?"+values.Encode(), "", &payload)
		data := payload[id]
		if data.Success {
			age, _ := strconv.Atoi(strings.Trim(string(data.Data.Age), `"`))
			if age > 0 && age <= 120 {
				result.Ratings = []string{strconv.Itoa(age) + "+"}
			}
			if prefs.Region == "DE" {
				if rating := strings.TrimSpace(data.Data.Boards["usk"].Rating); rating != "" && len(rating) < 30 {
					result.Ratings = []string{"USK " + rating}
				}
			}
			result.Note = limitDescription(data.Data.Descriptors.Notes)
		}
		result.Source = "Steam"
		result.Reference = "https://store.steampowered.com/app/" + id + "/"
	case (kind == "movie" || kind == "series") && (source == "imdb" || source == "tmdb"):
		result = screenAdvisory(ctx, kind, source, id, prefs)
	}
	reply(w, 200, result)
}

func screenAdvisory(ctx context.Context, kind, source, id string, prefs settings) titleAdvisory {
	var result titleAdvisory
	var imdb, tmdb string
	if source == "imdb" && imdbID.MatchString(id) {
		imdb = id
	}
	if source == "tmdb" && numericID.MatchString(id) {
		tmdb = id
	}
	if imdb != "" && prefs.TMDB != "" {
		var found struct {
			Movies []tmdbTitle `json:"movie_results"`
			Series []tmdbTitle `json:"tv_results"`
		}
		if getJSON(ctx, "https://api.themoviedb.org/3/find/"+imdb+"?external_source=imdb_id", prefs.TMDB, &found) == nil {
			titles := found.Movies
			if kind == "series" {
				titles = found.Series
			}
			if len(titles) == 1 && titles[0].ID > 0 {
				tmdb = strconv.Itoa(titles[0].ID)
			}
		}
	}
	if tmdb != "" && prefs.TMDB != "" {
		part := "release_dates"
		if kind == "series" {
			part = "content_ratings"
		}
		var data struct {
			Results []struct {
				Region string `json:"iso_3166_1"`
				Rating string
				Dates  []struct{ Certification string } `json:"release_dates"`
			}
		}
		if getJSON(ctx, "https://api.themoviedb.org/3/"+tmdbKind(kind)+"/"+tmdb+"/"+part, prefs.TMDB, &data) == nil {
			for _, row := range data.Results {
				if row.Region != prefs.Region {
					continue
				}
				values := []string{row.Rating}
				for _, date := range row.Dates {
					values = append(values, date.Certification)
				}
				for _, rating := range values {
					rating = strings.TrimSpace(rating)
					if rating != "" && len(rating) < 40 && !slices.Contains(result.Ratings, rating) {
						result.Ratings = append(result.Ratings, rating)
					}
				}
			}
			if len(result.Ratings) > 0 {
				result.Source = "TMDB"
			}
		}
		if imdb == "" {
			var external struct {
				IMDb string `json:"imdb_id"`
			}
			if getJSON(ctx, "https://api.themoviedb.org/3/"+tmdbKind(kind)+"/"+tmdb+"/external_ids", prefs.TMDB, &external) == nil && imdbID.MatchString(external.IMDb) {
				imdb = external.IMDb
			}
		}
	}
	if imdb != "" {
		result.Reference = "https://www.imdb.com/title/" + imdb + "/parentalguide/"
		if len(result.Ratings) == 0 {
			var data struct {
				Certificates []struct {
					Country struct{ Code string }
					Rating  string
				}
			}
			if getJSON(ctx, "https://api.imdbapi.dev/titles/"+imdb+"/certificates", "", &data) == nil {
				for _, item := range data.Certificates {
					rating := strings.TrimSpace(item.Rating)
					if item.Country.Code == prefs.Region && rating != "" && len(rating) < 40 && !slices.Contains(result.Ratings, rating) {
						result.Ratings = append(result.Ratings, rating)
					}
				}
				if len(result.Ratings) > 0 {
					result.Source = "IMDbAPI"
				}
			}
		}
		type warning struct{ Category, Severity string }
		var guide struct {
			ParentsGuide, Items, Categories []warning
		}
		if getJSON(ctx, "https://api.imdbapi.dev/titles/"+imdb+"/parentsGuide", "", &guide) == nil {
			for _, warning := range append(append(guide.ParentsGuide, guide.Items...), guide.Categories...) {
				category := strings.TrimSpace(warning.Category)
				severity := strings.TrimSpace(warning.Severity)
				if len(category) == 0 || len(category) > 100 || len(severity) == 0 || len(severity) > 50 || strings.EqualFold(severity, "none") {
					continue
				}
				label := category + " · " + severity
				if !slices.Contains(result.Warnings, label) {
					result.Warnings = append(result.Warnings, label)
				}
				if len(result.Warnings) == 12 {
					break
				}
			}
		}

	}
	return result
}
