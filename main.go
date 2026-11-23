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

const product = "Medatheke by Maximilian"
const storageName = "Medienregal"

var releaseRepo = "TheGrifer1337/Medienregal"
var version = "1.0"

//go:embed web/*
var assets embed.FS

type entry struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	Title    string   `json:"title"`
	Subtitle string   `json:"subtitle,omitempty"`
	Cover    string   `json:"cover,omitempty"`
	Genres   []string `json:"genres,omitempty"`
	Total    int      `json:"total"`
	Progress int      `json:"progress"`
	Status   string   `json:"status,omitempty"`
	Ratings  [3]int   `json:"ratings"` // Each unit represents half a star.
	Source   string   `json:"source,omitempty"`
}

type settings struct {
	TMDB      string   `json:"tmdb"`
	Watchmode string   `json:"watchmode"`
	Region    string   `json:"region"`
	Providers []string `json:"providers,omitempty"`
}

type library struct {
	Entries  []entry  `json:"entries"`
	Settings settings `json:"settings"`
}

type app struct {
	mu       sync.Mutex
	path     string
	data     library
	token    string
	exit     chan struct{}
	update   updateCheck
	lifetime context.Context
}

var statuses = map[string][]string{
	"movie":  {"seen", "want"},
	"series": {"seen", "started", "want"},
	"book":   {"read", "started", "finished", "want"},
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
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128*1024))
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
	if !decode(w, r, &item) {
		return
	}
	if r.Method == "PUT" && !valid(item) {
		fail(w, 400, "Prüfe Titel, Status, Fortschritt und Bewertung.")
		return
	}
	next := a.data
	next.Entries = make([]entry, 0, len(a.data.Entries)+1)
	found := false
	for _, old := range a.data.Entries {
		if old.ID == item.ID {
			found = true
			if r.Method == "PUT" {
				next.Entries = append(next.Entries, item)
			}
		} else {
			next.Entries = append(next.Entries, old)
		}
	}
	if !found && r.Method == "PUT" {
		next.Entries = append(next.Entries, item)
	}
	a.commit(w, next)
}

func (a *app) commit(w http.ResponseWriter, next library) {
	if err := writeLibrary(a.path, next); err != nil {
		fail(w, 500, "Die Änderungen konnten nicht gespeichert werden.")
		return
	}
	a.data = next
	reply(w, 200, true)
}

func (a *app) preferences(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if r.Method == "GET" {
		reply(w, 200, map[string]any{"version": version, "region": a.data.Settings.Region, "hasTmdb": a.data.Settings.TMDB != "", "hasWatchmode": a.data.Settings.Watchmode != "", "providers": a.data.Settings.Providers})
		return
	}
	var request struct {
		Token           string
		Watchmode       string
		Region          string
		Remove          bool
		RemoveWatchmode bool
		Providers       *[]string
	}
	if !decode(w, r, &request) {
		return
	}
	next := a.data
	if len(request.Region) != 2 || strings.ContainsFunc(request.Region, func(c rune) bool { return c < 'A' || c > 'Z' }) || len(request.Token) > 1024 || len(request.Watchmode) > 256 {
		fail(w, 400, "Prüfe Ländercode und TMDB-Token.")
		return
	}
	next.Settings.Region = request.Region
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

func (a *app) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/entries", a.entries)
	mux.HandleFunc("PUT /api/entries", a.entries)
	mux.HandleFunc("DELETE /api/entries", a.entries)
	mux.HandleFunc("GET /api/settings", a.preferences)
	mux.HandleFunc("PUT /api/settings", a.preferences)
	mux.HandleFunc("GET /api/search", a.search)
	mux.HandleFunc("GET /api/details", a.details)
	mux.HandleFunc("GET /api/streaming", a.streaming)
	mux.HandleFunc("GET /api/update", func(w http.ResponseWriter, r *http.Request) {
		reply(w, 200, a.update.snapshot())
	})
	mux.HandleFunc("POST /api/update", func(w http.ResponseWriter, r *http.Request) {
		reply(w, 202, a.update.start(a.lifetime))
	})
	mux.HandleFunc("POST /api/history/preview", a.previewHistory)
	mux.HandleFunc("POST /api/history/import", a.importHistory)
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
	web, _ := fs.Sub(assets, "web")
	mux.Handle("GET /", http.FileServerFS(web))
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
	a := &app{path: dataPath, data: data, token: hex.EncodeToString(key), exit: make(chan struct{}, 1), lifetime: lifetime,
		update: updateCheck{status: updateStatus{State: "idle"}}}
	address := listener.Addr().String()
	localURL := "http://" + address + "/" + a.token + "/"
	mux := http.StripPrefix("/"+a.token, a.routes())
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 35 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != address || !strings.HasPrefix(r.URL.Path, "/"+a.token+"/") || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != "http://"+address) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' https:; style-src 'self'; script-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		mux.ServeHTTP(w, r)
	})}
	go server.Serve(listener)
	if noLaunch {
		fmt.Println(localURL)
		<-a.exit
		return nil
	}
	a.update.start(lifetime)
	windowErr := runWindow(localURL, dataDir, a.exit)
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
	flag.Parse()
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
