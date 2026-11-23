package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const product = "Mediathek by Maximilian"
const storageName = "Medienregal"

var releaseRepo = "TheGrifer1337/Medienregal"
var version = "1.0"

//go:embed web/*
var assets embed.FS

type entry struct {
	ID            string     `json:"id"`
	Revision      string     `json:"revision,omitempty"`
	Kind          string     `json:"kind"`
	Title         string     `json:"title"`
	Subtitle      string     `json:"subtitle,omitempty"`
	Series        string     `json:"series,omitempty"`
	Sequence      float64    `json:"sequence,omitempty"`
	Loan          *mediaLoan `json:"loan,omitempty"`
	Trailer       string     `json:"trailer,omitempty"`
	IMDb          float64    `json:"imdb,omitempty"`
	Cover         string     `json:"cover,omitempty"`
	CustomCover   string     `json:"customCover,omitempty"`
	Favorite      bool       `json:"favorite,omitempty"`
	Genres        []string   `json:"genres,omitempty"`
	Total         int        `json:"total"`
	Progress      int        `json:"progress"`
	Status        string     `json:"status,omitempty"`
	Ratings       [3]int     `json:"ratings"` // Each unit represents half a star.
	Source        string     `json:"source,omitempty"`
	CatalogID     string     `json:"catalogId,omitempty"`
	Aliases       []string   `json:"aliases,omitempty"`
	Description   string     `json:"description,omitempty"`
	Notes         string     `json:"notes,omitempty"`
	Lists         []string   `json:"lists,omitempty"`
	CompletedYear int        `json:"completedYear,omitempty"`
	launchData    string
	ImportEdited  bool `json:"importEdited,omitempty"`
}

type settings struct {
	SteamUser string   `json:"steamUser,omitempty"`
	SteamKey  string   `json:"steamKey,omitempty"`
	TMDB      string   `json:"tmdb"`
	Watchmode string   `json:"watchmode"`
	Language  string   `json:"language,omitempty"`
	Region    string   `json:"region"`
	Providers []string `json:"providers,omitempty"`
	View      string   `json:"view,omitempty"`
}

type library struct {
	Entries           []entry                `json:"entries"`
	Settings          settings               `json:"settings"`
	Goals             map[int]map[string]int `json:"goals,omitempty"`
	LastHistoryImport []historyChange        `json:"lastHistoryImport,omitempty"`
	Lists             []string               `json:"lists,omitempty"`
	Undo              *collectionUndo        `json:"undo,omitempty"`
}

type app struct {
	mu             sync.Mutex
	path           string
	data           library
	token          string
	exit           chan struct{}
	update         updateCheck
	lifetime       context.Context
	fullscreen     chan bool
	launcherGames  map[string]entry
	hosted         bool
	remote         desktopAccount
	host           *accountHost
	hostHTTP       *http.Server
	accountActions sync.Mutex
}

var statuses = map[string][]string{
	"movie":  {"seen", "want", "started"},
	"series": {"seen", "started", "want"},
	"book":   {"read", "started", "finished", "want"},
	"game":   {"started", "finished", "want", "library"},
}

func readLibrary(path string) (library, error) {
	data := library{Entries: []entry{}, Settings: settings{Region: "DE"}}
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return data, nil
	}
	if err != nil {
		return data, err
	}
	err = json.Unmarshal(content, &data)
	return data, err
}

func writeLibrary(path string, data library) error {
	return writeJSON(path, data)
}

func writeJSON(path string, data any) error {
	file, err := os.CreateTemp(filepath.Dir(path), "regal-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	err = json.NewEncoder(file).Encode(data)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), path)
}

func valid(item entry) bool {
	if item.ID == "" || len(item.ID) > 200 || strings.TrimSpace(item.Title) == "" || len(item.Title) > 600 {
		return false
	}
	if len(item.Notes) > 16000 || len(item.Lists) > 100 || len(item.Description) > 20000 || !validCustomCover(item.CustomCover) {
		return false
	}
	if !validMediaExtras(item) {
		return false
	}
	if item.Total < 0 || item.Progress < 0 || (item.Total > 0 && item.Progress > item.Total) {
		return false
	}
	for _, rating := range item.Ratings {
		if rating < 0 || rating > 10 {
			return false
		}
	}
	for _, status := range statuses[item.Kind] {
		if item.Status == status {
			return true
		}
	}
	return false
}

func reply(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}

func fail(w http.ResponseWriter, code int, message string) {
	reply(w, code, map[string]string{"error": message})
}

func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	return decodeLimited(w, r, target, 128*1024)
}

func decodeLimited(w http.ResponseWriter, r *http.Request, target any, limit int64) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	if err := decoder.Decode(target); err != nil || decoder.Decode(new(any)) != io.EOF {
		fail(w, 400, "Die Eingabe konnte nicht gelesen werden.")
		return false
	}
	return true
}

func (a *app) entries(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if r.Method == "GET" {
		reply(w, 200, a.data.Entries)
		return
	}
	var item entry
	if !decodeLimited(w, r, &item, 512*1024) {
		return
	}
	if r.Method == "PUT" && !valid(item) {
		fail(w, 400, "Prüfe Titel, Status, Fortschritt und Bewertung.")
		return
	}
	if r.Method == "DELETE" {
		next := a.data
		previous := findEntry(next.Entries, item.ID)
		if previous != nil {
			if a.hosted && previous.Revision != item.Revision {
				fail(w, 409, "Dieser Titel wurde auf einem anderen Gerät geändert. Öffne ihn erneut.")
				return
			}
			next.Entries = replaceEntries(next.Entries, []entry{*previous}, nil)
			next.Undo = &collectionUndo{Before: []entry{*previous}, Label: "Titel entfernt"}
		}
		a.commit(w, next)
		return
	}
	next := a.data
	previous := findEntry(next.Entries, item.ID)
	if a.hosted && previous == nil && item.Revision != "" {
		fail(w, 409, "Dieser Titel wurde auf einem anderen Gerät geändert. Öffne ihn erneut.")
		return
	}
	item.Lists = canonicalLists(item.Lists, next.Lists)
	setCompletion(&item, previous)
	var before []entry
	if previous != nil {
		if a.hosted && previous.Revision != item.Revision {
			fail(w, 409, "Dieser Titel wurde auf einem anderen Gerät geändert. Öffne ihn erneut.")
			return
		}
		if equalEntry(*previous, item) {
			reply(w, 200, true)
			return
		}
		if previous.Source == "Streamingverlauf" {
			item.ImportEdited = true
		}
		before = []entry{*previous}
	}
	next.Entries = replaceEntries(next.Entries, before, []entry{item})
	next.Undo = &collectionUndo{Before: before, After: []entry{item}, Label: "Titel gespeichert"}
	a.commit(w, next)
}

// persist is called while a.mu is held. Hosted edits carry revision checks for title forms.
func (a *app) persist(next library) error {
	if a.hosted {
		previous := make(map[string]entry, len(a.data.Entries))
		for _, item := range a.data.Entries {
			previous[item.ID] = item
		}
		next.Entries = append([]entry{}, next.Entries...)
		for i, item := range next.Entries {
			old, ok := previous[item.ID]
			if ok && equalEntry(old, item) {
				next.Entries[i].Revision = old.Revision
				continue
			}
			revision, err := randomToken()
			if err != nil {
				return err
			}
			next.Entries[i].Revision = revision
		}
	}
	if err := writeLibrary(a.path, next); err != nil {
		return err
	}
	a.data = next
	return nil
}

func (a *app) commit(w http.ResponseWriter, next library) {
	if err := a.persist(next); err != nil {
		fail(w, 500, "Die Änderungen konnten nicht gespeichert werden.")
		return
	}
	reply(w, 200, true)
}

func (a *app) preferences(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if r.Method == "GET" {
		reply(w, 200, map[string]any{"version": version, "region": a.data.Settings.Region, "steamUser": a.data.Settings.SteamUser, "hasSteam": a.data.Settings.SteamKey != "", "language": a.data.Settings.locale(), "hasTmdb": a.data.Settings.TMDB != "", "hasWatchmode": a.data.Settings.Watchmode != "", "providers": a.data.Settings.Providers, "view": a.data.Settings.View, "chartGenres": chartGenreOptions()})
		return
	}
	var request struct {
		SteamUser, SteamKey *string
		RemoveSteam         bool
		Token               string
		Watchmode           string
		Region              string
		Language            string
		Remove              bool
		RemoveWatchmode     bool
		Providers           *[]string
	}
	if !decode(w, r, &request) {
		return
	}
	next := a.data
	if len(request.Region) != 2 || strings.ContainsFunc(request.Region, func(c rune) bool { return c < 'A' || c > 'Z' }) || len(request.Token) > 1024 || len(request.Watchmode) > 256 {
		fail(w, 400, "Prüfe Ländercode und TMDB-Token.")
		return
	}
	if request.SteamUser != nil {
		id := strings.TrimSpace(*request.SteamUser)
		if id != "" && !steamUserID.MatchString(id) {
			fail(w, 400, "Enter a 17-digit SteamID64.")
			return
		}
		next.Settings.SteamUser = id
	}
	if request.RemoveSteam {
		next.Settings.SteamKey = ""
	} else if request.SteamKey != nil && strings.TrimSpace(*request.SteamKey) != "" {
		key := strings.TrimSpace(*request.SteamKey)
		if !steamAPIKey.MatchString(key) {
			fail(w, 400, "Enter a valid Steam Web API key.")
			return
		}
		next.Settings.SteamKey = key
	}
	next.Settings.Region = request.Region
	if request.Language != "" {
		switch request.Language {
		case "de", "en", "zh-Hans":
			next.Settings.Language = request.Language
		default:
			fail(w, 400, "Unsupported interface language.")
			return
		}
	}
	if request.Providers != nil {
		if len(*request.Providers) > 100 {
			fail(w, 400, "Zu viele Streaminganbieter ausgewählt.")
			return
		}
		providers := []string{}
		seen := map[string]bool{}
		for _, value := range *request.Providers {
			name := strings.Join(strings.Fields(value), " ")
			if len(name) > 120 {
				fail(w, 400, "Ein Name der Streaminganbieter ist zu lang.")
				return
			}
			key := strings.ToLower(name)
			if key != "" && !seen[key] {
				providers = append(providers, name)
				seen[key] = true
			}
		}
		next.Settings.Providers = providers
	}
	switch {
	case request.Remove:
		next.Settings.TMDB = ""
	case strings.TrimSpace(request.Token) != "":
		next.Settings.TMDB = strings.TrimSpace(request.Token)
	}
	switch {
	case request.RemoveWatchmode:
		next.Settings.Watchmode = ""
	case strings.TrimSpace(request.Watchmode) != "":
		next.Settings.Watchmode = strings.TrimSpace(request.Watchmode)
	}
	a.commit(w, next)
}

func (a *app) getSettings() settings {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.data.Settings
}

var accountWeb = func() http.Handler {
	web, _ := fs.Sub(assets, "web")
	return http.FileServerFS(web)
}()

func (a *app) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/account", a.desktopAccount)
	mux.HandleFunc("POST /api/account", a.desktopAccount)
	mux.HandleFunc("PUT /api/account/import", a.importLocalLibrary)
	mux.HandleFunc("PUT /api/account/installed-games", a.importAccountGames)
	mux.HandleFunc("GET /api/entries", a.entries)
	mux.HandleFunc("PUT /api/entries", a.entries)
	mux.HandleFunc("DELETE /api/entries", a.entries)
	mux.HandleFunc("GET /api/settings", a.preferences)
	mux.HandleFunc("PUT /api/settings", a.preferences)
	mux.HandleFunc("GET /api/search", a.search)
	mux.HandleFunc("GET /api/details", a.details)
	mux.HandleFunc("GET /api/cover", a.lookupCover)
	mux.HandleFunc("GET /api/advisories", a.advisories)
	mux.HandleFunc("GET /api/steam-info", a.steamInfo)
	mux.HandleFunc("GET /api/imdb-rating", a.imdbRating)
	mux.HandleFunc("GET /api/movie-trailer", a.movieTrailer)
	mux.HandleFunc("GET /api/movie-series", a.movieSeries)
	mux.HandleFunc("GET /api/goals", a.goals)
	mux.HandleFunc("PUT /api/goals", a.goals)
	mux.HandleFunc("GET /api/barcode", a.barcodeLookup)
	mux.HandleFunc("POST /api/barcode", a.scanBarcode)
	mux.HandleFunc("POST /api/launch-game", a.launchGame)
	mux.HandleFunc("GET /api/streaming", a.streaming)
	mux.HandleFunc("GET /api/collection", a.collection)
	mux.HandleFunc("GET /api/launchers", a.previewLaunchers)
	mux.HandleFunc("POST /api/launchers", a.importLaunchers)
	mux.HandleFunc("PUT /api/collection/favorite", a.favoriteEntry)
	mux.HandleFunc("PUT /api/lists", a.customList)
	mux.HandleFunc("PUT /api/view", a.collectionView)
	mux.HandleFunc("POST /api/collection/undo", a.undoCollection)
	mux.HandleFunc("POST /api/collection/merge", a.mergeCollection)
	mux.HandleFunc("PUT /api/collection/enrich", a.enrichEntry)
	mux.HandleFunc("GET /api/rankings", a.rankings)
	mux.HandleFunc("POST /api/fullscreen", func(w http.ResponseWriter, r *http.Request) {
		var request struct{ Enabled bool }
		if !decode(w, r, &request) {
			return
		}
		if a.fullscreen != nil {
			select {
			case a.fullscreen <- request.Enabled:
			case <-r.Context().Done():
			}
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/update", func(w http.ResponseWriter, r *http.Request) {
		reply(w, 200, a.update.snapshot())
	})
	mux.HandleFunc("POST /api/update", func(w http.ResponseWriter, r *http.Request) {
		reply(w, 202, a.update.start(a.lifetime))
	})
	mux.HandleFunc("POST /api/history/preview", a.previewHistory)
	mux.HandleFunc("POST /api/history/import", a.importHistory)
	mux.HandleFunc("GET /api/history/undo", a.historyUndo)
	mux.HandleFunc("POST /api/history/undo", a.historyUndo)
	mux.HandleFunc("POST /api/open-link", func(w http.ResponseWriter, r *http.Request) {
		var request struct{ URL string }
		if !decode(w, r, &request) {
			return
		}
		address, err := url.Parse(request.URL)
		if err != nil || address.Scheme != "https" || address.Hostname() == "" || address.User != nil || len(request.URL) > 4000 {
			fail(w, 400, "Ungültiger Link.")
			return
		}
		if err := openExternal(address.String()); err != nil {
			fail(w, 500, "Der Link konnte nicht geöffnet werden.")
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /api/exit", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
		select {
		case a.exit <- struct{}{}:
		default:
		}
	})
	mux.Handle("GET /", accountWeb)
	return mux
}

func serve(noLaunch bool, dataDir string) error {
	if dataDir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		dataDir = filepath.Join(base, storageName)
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return err
	}
	dataPath := filepath.Join(dataDir, "sammlung.json")
	data, err := readLibrary(dataPath)
	if err != nil {
		return fmt.Errorf("Sammlung konnte nicht geladen werden: %w", err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	key := make([]byte, 24)
	if _, err = rand.Read(key); err != nil {
		return err
	}
	lifetime, stop := context.WithCancel(context.Background())
	defer stop()
	a := &app{path: dataPath, data: data, token: hex.EncodeToString(key), exit: make(chan struct{}, 1), lifetime: lifetime, fullscreen: make(chan bool, 8),
		update: updateCheck{status: updateStatus{State: "idle"}}}
	address := listener.Addr().String()
	localURL := "http://" + address + "/" + a.token + "/"
	a.remote = readDesktopAccount(dataDir)
	if _, err := os.Stat(filepath.Join(dataDir, "account-host.enabled")); err == nil {
		a.host, a.hostHTTP, _ = listenAccountHost(filepath.Join(dataDir, "Kontoserver"))
	}
	defer func() {
		a.mu.Lock()
		hostHTTP := a.hostHTTP
		a.mu.Unlock()
		if hostHTTP != nil {
			_ = hostHTTP.Close()
		}
	}()
	mux := http.StripPrefix("/"+a.token, a.withAccountProxy(a.routes()))
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 35 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != address || !strings.HasPrefix(r.URL.Path, "/"+a.token+"/") || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != "http://"+address) {
			http.NotFound(w, r)
			return
		}
		accountHeaders(w)
		mux.ServeHTTP(w, r)
	})}
	go server.Serve(listener)
	if noLaunch {
		fmt.Println(localURL)
		<-a.exit
		return nil
	}
	a.update.start(lifetime)
	windowErr := runWindow(localURL, dataDir, a.exit, a.fullscreen)
	stop()
	shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdown)
	if err := a.update.apply(); err != nil && windowErr == nil {
		return err
	}
	return windowErr
}

func main() {
	noLaunch := flag.Bool("serve", false, "Start local test server without opening a window")
	dataDir := flag.String("data", "", "Path for personal data")
	hostAccounts := flag.Bool("account-host", false, "Serve Mediathek accounts on 127.0.0.1:8765")
	flag.Parse()
	if *hostAccounts {
		folder := *dataDir
		if folder == "" {
			base, err := os.UserConfigDir()
			if err != nil {
				showError(err.Error())
				return
			}
			folder = filepath.Join(base, storageName, "Kontoserver")
		}
		_, server, err := listenAccountHost(folder)
		if err != nil {
			showError(err.Error())
			return
		}
		defer server.Close()
		log.Printf("Mediathek account server: http://%s/ (invite: %s)", accountPort, filepath.Join(folder, "invite.txt"))
		select {}
	}
	if !*noLaunch {
		release, err := lockInstance()
		if err != nil {
			showError(err.Error())
			return
		}
		defer release()
	}
	if err := serve(*noLaunch, *dataDir); err != nil {
		if *noLaunch {
			log.Fatal(err)
		}
		showError(err.Error())
	}
}
