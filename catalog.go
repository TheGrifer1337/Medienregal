package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var webClient = &http.Client{Timeout: 10 * time.Second}
var imdbID = regexp.MustCompile(`^tt[0-9]+$`)
var editionID = regexp.MustCompile(`^OL[0-9]+M$`)
var numericID = regexp.MustCompile(`^[0-9]+$`)

func getJSON(ctx context.Context, address, bearer string, target any) error {
	request, err := http.NewRequestWithContext(ctx, "GET", address, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "Medienregal/"+version+" (desktop application)")
	request.Header.Set("Accept", "application/json")
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	response, err := webClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", response.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 12*1024*1024)).Decode(target)
}

func prettyGenres(values []string) []string {
	names := map[string]string{
		"action": "Action", "adventure": "Abenteuer", "animation": "Animation",
		"comedy": "Komödie", "crime": "Krimi", "documentary": "Dokumentation",
		"family": "Familie", "fantasy": "Fantasy", "fiction": "Belletristik",
		"history": "Geschichte", "horror": "Horror", "manga": "Manga", "anime": "Anime",
		"music": "Musik", "mystery": "Mystery", "romance": "Romantik", "romantic": "Romantik",
		"sci-fi": "Sci-Fi", "science fiction": "Sci-Fi", "science-fiction": "Sci-Fi",
		"supernatural": "Übernatürlich", "thriller": "Thriller", "war": "Krieg",
	}
	result := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if translated := names[key]; translated != "" {
			value = translated
		}
		key = strings.ToLower(value)
		if value == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, value)
	}
	return result
}

type cinemeta struct {
	ID          string `json:"imdb_id"`
	Name        string
	Poster      string
	ReleaseInfo string
	Genre       []string
	Country     string
	Videos      []struct {
		Season  int
		Episode int
	}
}

func (movie cinemeta) internal(kind string) entry {
	genres := movie.Genre
	if strings.Contains(strings.ToLower(movie.Country), "japan") {
		for _, genre := range genres {
			if genre == "Animation" {
				genres = append(genres, "Anime")
				break
			}
		}
	}
	episodes := 0
	if kind == "series" {
		seen := map[[2]int]bool{}
		for _, video := range movie.Videos {
			if video.Season > 0 && video.Episode > 0 {
				seen[[2]int{video.Season, video.Episode}] = true
			}
		}
		episodes = len(seen)
	}
	return entry{ID: kind + ":imdb:" + movie.ID, Kind: kind, Title: movie.Name, Subtitle: movie.ReleaseInfo, Cover: movie.Poster, Genres: prettyGenres(genres), Total: episodes, Source: "Cinemeta"}
}

func searchCinemeta(ctx context.Context, kind, query string) ([]entry, error) {
	var payload struct{ Metas []cinemeta }
	address := "https://v3-cinemeta.strem.io/catalog/" + kind + "/top/search=" + url.PathEscape(query) + ".json"
	if err := getJSON(ctx, address, "", &payload); err != nil {
		return nil, err
	}
	items := []entry{}
	for _, item := range payload.Metas {
		if imdbID.MatchString(item.ID) && item.Name != "" {
			items = append(items, item.internal(kind))
		}
		if len(items) == 25 {
			break
		}
	}
	return items, nil
}

type tmdbTitle struct {
	ID      int
	Title   string
	Name    string
	Poster  string `json:"poster_path"`
	Release string `json:"release_date"`
	AirDate string `json:"first_air_date"`
	Total   int    `json:"number_of_episodes"`
	Genres  []struct{ Name string }
}

func tmdbKind(kind string) string {
	if kind == "series" {
		return "tv"
	}
	return "movie"
}

func (title tmdbTitle) internal(kind string) entry {
	name, date := title.Title, title.Release
	if kind == "series" {
		name, date = title.Name, title.AirDate
	}
	if len(date) > 4 {
		date = date[:4]
	}
	cover := ""
	if strings.HasPrefix(title.Poster, "/") {
		cover = "https://image.tmdb.org/t/p/w500" + title.Poster
	}
	genres := []string{}
	for _, genre := range title.Genres {
		genres = append(genres, genre.Name)
	}
	return entry{ID: kind + ":tmdb:" + strconv.Itoa(title.ID), Kind: kind, Title: name, Subtitle: date, Cover: cover, Total: title.Total, Genres: prettyGenres(genres), Source: "TMDB"}
}

func searchTMDB(ctx context.Context, kind, query, token string) ([]entry, error) {
	var payload struct{ Results []tmdbTitle }
	address := "https://api.themoviedb.org/3/search/" + tmdbKind(kind) + "?language=de-DE&include_adult=false&query=" + url.QueryEscape(query)
	if err := getJSON(ctx, address, token, &payload); err != nil {
		return nil, err
	}
	items := []entry{}
	for _, item := range payload.Results {
		if item.ID > 0 {
			items = append(items, item.internal(kind))
		}
	}
	return items, nil
}

type bookEdition struct {
	Key        string
	Title      string
	Publishers []string
	Covers     []int
	Pages      int `json:"number_of_pages"`
}

func bookCover(id int) string {
	if id <= 0 {
		return ""
	}
	return fmt.Sprintf("https://covers.openlibrary.org/b/id/%d-M.jpg?default=false", id)
}

func searchBooks(ctx context.Context, query url.Values) ([]entry, error) {
	params := url.Values{"limit": {"22"}, "fields": {"key,title,author_name,cover_i,number_of_pages_median,subject,editions,editions.key,editions.title,editions.publisher,editions.cover_i,editions.number_of_pages"}}
	title := strings.TrimSpace(query.Get("q"))
	author := strings.TrimSpace(query.Get("author"))
	publisher := strings.TrimSpace(query.Get("publisher"))
	volume := strings.TrimSpace(query.Get("volume"))
	isbn := strings.NewReplacer("-", "", " ", "").Replace(title)
	if (len(isbn) == 10 || len(isbn) == 13) && numericID.MatchString(strings.TrimSuffix(strings.ToUpper(isbn), "X")) {
		params.Set("isbn", isbn)
	} else if title != "" {
		params.Set("q", title)
	}
	if author != "" {
		params.Set("author", author)
	}
	if publisher != "" {
		params.Set("publisher", publisher)
	}
	if volume != "" {
		s := strings.TrimSpace(params.Get("q") + " " + volume)
		params.Set("q", s)
	}
	var payload struct {
		Docs []struct {
			Key      string
			Title    string
			Authors  []string `json:"author_name"`
			Cover    int      `json:"cover_i"`
			Median   int      `json:"number_of_pages_median"`
			Subjects []string `json:"subject"`
			Editions struct {
				Docs []struct {
					Key       string
					Title     string
					Publisher []string
					Cover     int `json:"cover_i"`
					Pages     int `json:"number_of_pages"`
				}
			}
		}
	}
	if err := getJSON(ctx, "https://openlibrary.org/search.json?"+params.Encode(), "", &payload); err != nil {
		return nil, err
	}
	items := []entry{}
	for _, book := range payload.Docs {
		genres := []string{}
		for _, subject := range book.Subjects {
			lower := strings.ToLower(subject)
			if strings.Contains(lower, "manga") {
				genres = append(genres, "Manga")
			}
			for _, name := range []string{"Fantasy", "Horror", "Science Fiction", "Mystery", "Romance", "Thriller", "Crime", "History"} {
				if strings.Contains(lower, strings.ToLower(name)) {
					genres = append(genres, name)
				}
			}
		}
		base := entry{ID: "book:open:" + book.Key, Kind: "book", Title: book.Title, Subtitle: strings.Join(book.Authors, ", "), Cover: bookCover(book.Cover), Genres: prettyGenres(genres), Total: book.Median, Source: "Open Library"}
		added := false
		for _, edition := range book.Editions.Docs {
			id := strings.TrimPrefix(edition.Key, "/books/")
			if !editionID.MatchString(id) {
				continue
			}
			item := base
			item.ID = "book:ol:" + id
			if edition.Title != "" {
				item.Title = edition.Title
			}
			if edition.Cover > 0 {
				item.Cover = bookCover(edition.Cover)
			}
			item.Total = edition.Pages
			if len(edition.Publisher) > 0 {
				item.Subtitle = strings.Trim(strings.Join([]string{base.Subtitle, edition.Publisher[0]}, " · "), " ·")
			}
			items = append(items, item)
			added = true
		}
		if !added && base.Title != "" {
			items = append(items, base)
		}
	}
	if len(items) == 0 && params.Get("isbn") != "" {
		var edition bookEdition
		if err := getJSON(ctx, "https://openlibrary.org/isbn/"+url.PathEscape(isbn)+".json", "", &edition); err == nil && edition.Title != "" {
			cover := ""
			if len(edition.Covers) > 0 {
				cover = bookCover(edition.Covers[0])
			}
			items = append(items, entry{ID: "book:ol:" + strings.TrimPrefix(edition.Key, "/books/"), Kind: "book", Title: edition.Title, Subtitle: strings.Join(edition.Publishers, ", "), Cover: cover, Total: edition.Pages, Source: "Open Library"})
		}
	}
	return items, nil
}

func (a *app) search(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	kind, query := params.Get("kind"), strings.TrimSpace(params.Get("q"))
	if _, ok := statuses[kind]; !ok || len(r.URL.RawQuery) > 1700 || (query == "" && (kind != "book" || (params.Get("author") == "" && params.Get("publisher") == "" && params.Get("volume") == ""))) {
		fail(w, 400, "Gib einen Suchbegriff ein.")
		return
	}
	prefs := a.getSettings()
	var items []entry
	var err error
	switch {
	case kind == "book":
		items, err = combinedBooks(r.Context(), params)
	case prefs.TMDB != "":
		items, err = searchTMDB(r.Context(), kind, query, prefs.TMDB)
		if err != nil || len(items) == 0 {
			items, err = searchCinemeta(r.Context(), kind, query)
		}
	default:
		items, err = searchCinemeta(r.Context(), kind, query)
	}
	if err != nil {
		fail(w, 502, "Der Katalog ist gerade nicht erreichbar. Du kannst den Titel manuell hinzufügen.")
		return
	}
	reply(w, 200, items)
}

func (a *app) details(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	parts := strings.Split(id, ":")
	if len(parts) != 3 {
		fail(w, 400, "Ungültiger Katalogeintrag.")
		return
	}
	kind, source, key := parts[0], parts[1], parts[2]
	ctx := r.Context()
	var found entry
	err := errors.New("Kein Katalogeintrag")
	switch {
	case source == "imdb" && imdbID.MatchString(key) && (kind == "movie" || kind == "series"):
		var payload struct{ Meta cinemeta }
		err = getJSON(ctx, "https://v3-cinemeta.strem.io/meta/"+kind+"/"+key+".json", "", &payload)
		found = payload.Meta.internal(kind)
	case source == "tmdb" && numericID.MatchString(key) && (kind == "movie" || kind == "series"):
		var payload tmdbTitle
		err = getJSON(ctx, "https://api.themoviedb.org/3/"+tmdbKind(kind)+"/"+key+"?language=de-DE", a.getSettings().TMDB, &payload)
		found = payload.internal(kind)
	case kind == "book" && source == "ol" && editionID.MatchString(key):
		var payload bookEdition
		err = getJSON(ctx, "https://openlibrary.org/books/"+key+".json", "", &payload)
		found.Total = payload.Pages
		if len(payload.Covers) > 0 {
			found.Cover = bookCover(payload.Covers[0])
		}
	}
	if err != nil {
		fail(w, 502, "Detaildaten sind gerade nicht verfügbar.")
		return
	}
	reply(w, 200, found)
}
