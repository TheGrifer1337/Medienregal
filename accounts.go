package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
)

const accountCookie = "__Host-mediathek"
const accountPort = "127.0.0.1:8765"

var accountName = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{2,31}$`)
var opaqueToken = regexp.MustCompile(`^[a-f0-9]{64}$`)

type storedSession struct {
	Name    string
	Expires time.Time
}

type accountIndex struct {
	Users    map[string]string
	Sessions map[string]storedSession
}

type accountHost struct {
	mu             sync.Mutex
	folder, invite string
	index          accountIndex
	accounts       map[string]http.Handler
	attempts       map[string]loginAttempts
	passwords      chan struct{}
	dummyPassword  []byte
}

type loginAttempts struct {
	Count int
	Until time.Time
}

type accountRequest struct {
	Action, Server, Name, Password, Invite string
	Import, Native                         bool
}

func randomToken() (string, error) {
	var data [32]byte
	_, err := rand.Read(data[:])
	return hex.EncodeToString(data[:]), err
}

func sessionHash(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func hashAccountPassword(password string) ([]byte, error) {
	var salt [16]byte
	if _, err := rand.Read(salt[:]); err != nil {
		return nil, err
	}
	digest := argon2.IDKey([]byte(password), salt[:], 2, 19*1024, 1, 32)
	return []byte(hex.EncodeToString(salt[:]) + hex.EncodeToString(digest)), nil
}

func verifyAccountPassword(encoded []byte, password string) bool {
	hash, err := hex.DecodeString(string(encoded))
	if err != nil || len(hash) != 48 {
		return false
	}
	digest := argon2.IDKey([]byte(password), hash[:16], 2, 19*1024, 1, 32)
	return subtle.ConstantTimeCompare(hash[16:], digest) == 1
}

func openAccountHost(folder string) (*accountHost, error) {
	if err := os.MkdirAll(folder, 0700); err != nil {
		return nil, err
	}
	host := &accountHost{folder: folder, accounts: make(map[string]http.Handler), attempts: make(map[string]loginAttempts), passwords: make(chan struct{}, 2)}
	host.index = accountIndex{Users: make(map[string]string), Sessions: make(map[string]storedSession)}
	invitePath := filepath.Join(folder, "invite.txt")
	invite, err := os.ReadFile(invitePath)
	if errors.Is(err, os.ErrNotExist) {
		value, err := randomToken()
		if err != nil {
			return nil, err
		}
		if err = os.WriteFile(invitePath, []byte(value+"\n"), 0600); err != nil {
			return nil, err
		}
		invite = []byte(value)
	} else if err != nil {
		return nil, err
	}
	host.invite = strings.TrimSpace(string(invite))
	if !opaqueToken.MatchString(host.invite) {
		return nil, errors.New("invalid account invite file")
	}
	data, err := os.ReadFile(filepath.Join(folder, "accounts.json"))
	if err == nil {
		if err = json.Unmarshal(data, &host.index); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if host.index.Users == nil || host.index.Sessions == nil {
		return nil, errors.New("invalid accounts index")
	}
	dummy, err := randomToken()
	if err != nil {
		return nil, err
	}
	host.dummyPassword, err = hashAccountPassword(dummy)
	return host, err
}

func (h *accountHost) saveIndex() error {
	return writeJSON(filepath.Join(h.folder, "accounts.json"), h.index)
}

func (h *accountHost) permitLogin(key string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	if len(h.attempts) >= 4000 {
		for name, attempt := range h.attempts {
			if now.After(attempt.Until) {
				delete(h.attempts, name)
			}
		}
		if len(h.attempts) >= 4000 {
			return false
		}
	}
	for _, name := range []string{key, "*"} {
		old := h.attempts[name]
		limit := 12
		if name == "*" {
			limit = 120
		}
		if now.Before(old.Until) && old.Count >= limit {
			return false
		}
	}
	for _, name := range []string{key, "*"} {
		old := h.attempts[name]
		if now.After(old.Until) {
			old = loginAttempts{Until: now.Add(5 * time.Minute)}
		}
		old.Count++
		h.attempts[name] = old
	}
	return true
}

func (h *accountHost) authenticate(w http.ResponseWriter, r *http.Request) {
	var request accountRequest
	if !decodeLimited(w, r, &request, 8192) {
		return
	}
	request.Name = strings.ToLower(strings.TrimSpace(request.Name))
	if !accountName.MatchString(request.Name) || len(request.Password) < 10 || len(request.Password) > 256 || (request.Action != "login" && request.Action != "register") {
		fail(w, 400, "Benutzername oder Passwort hat kein gültiges Format.")
		return
	}
	if !h.permitLogin(request.Name) {
		fail(w, http.StatusTooManyRequests, "Zu viele Anmeldeversuche. Warte einige Minuten.")
		return
	}
	select {
	case h.passwords <- struct{}{}:
		defer func() { <-h.passwords }()
	default:
		fail(w, http.StatusTooManyRequests, "Der Kontoserver ist ausgelastet. Versuche es erneut.")
		return
	}
	var hash string
	if request.Action == "register" {
		if subtle.ConstantTimeCompare([]byte(request.Invite), []byte(h.invite)) != 1 {
			fail(w, http.StatusForbidden, "Der Einladungscode ist ungültig.")
			return
		}
		result, err := hashAccountPassword(request.Password)
		if err != nil {
			fail(w, 500, "Konto konnte nicht erstellt werden.")
			return
		}
		hash = string(result)
	} else {
		h.mu.Lock()
		known := h.index.Users[request.Name]
		h.mu.Unlock()
		check := []byte(known)
		if known == "" {
			check = h.dummyPassword
		}
		if !verifyAccountPassword(check, request.Password) || known == "" {
			fail(w, http.StatusUnauthorized, "Benutzername oder Passwort ist falsch.")
			return
		}
	}
	token, err := randomToken()
	if err != nil {
		fail(w, 500, "Anmeldung fehlgeschlagen.")
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if request.Action == "register" {
		if _, exists := h.index.Users[request.Name]; exists {
			fail(w, http.StatusConflict, "Dieser Benutzername ist bereits vergeben.")
			return
		}
		h.index.Users[request.Name] = hash
	}
	for key, value := range h.index.Sessions {
		if time.Now().After(value.Expires) {
			delete(h.index.Sessions, key)
		}
	}
	expires := time.Now().Add(30 * 24 * time.Hour)
	digest := sessionHash(token)
	h.index.Sessions[digest] = storedSession{request.Name, expires}
	if err = h.saveIndex(); err != nil {
		delete(h.index.Sessions, digest)
		if request.Action == "register" {
			delete(h.index.Users, request.Name)
		}
		fail(w, 500, "Anmeldung fehlgeschlagen.")
		return
	}
	if request.Native {
		reply(w, 200, map[string]string{"name": request.Name, "token": token})
		return
	}
	http.SetCookie(w, &http.Cookie{Name: accountCookie, Value: token, Path: "/", Expires: expires, HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
	reply(w, 200, map[string]string{"name": request.Name})
}

func (h *accountHost) identify(r *http.Request) (string, string) {
	token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == "" {
		if cookie, err := r.Cookie(accountCookie); err == nil {
			token = cookie.Value
		}
	}
	if !opaqueToken.MatchString(token) {
		return "", ""
	}
	key := sessionHash(token)
	h.mu.Lock()
	session := h.index.Sessions[key]
	h.mu.Unlock()
	if time.Now().After(session.Expires) {
		return "", ""
	}
	return session.Name, key
}

func (h *accountHost) logout(w http.ResponseWriter, r *http.Request) {
	_, key := h.identify(r)
	if key != "" {
		h.mu.Lock()
		old := h.index.Sessions[key]
		delete(h.index.Sessions, key)
		err := h.saveIndex()
		if err != nil {
			h.index.Sessions[key] = old
		}
		h.mu.Unlock()
		if err != nil {
			fail(w, 500, "Abmeldung fehlgeschlagen.")
			return
		}
	}
	http.SetCookie(w, &http.Cookie{Name: accountCookie, Path: "/", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
	w.WriteHeader(http.StatusNoContent)
}

func (h *accountHost) account(name string) (http.Handler, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if handler := h.accounts[name]; handler != nil {
		return handler, nil
	}
	file := filepath.Join(h.folder, "collection-"+name+".json")
	data, err := readLibrary(file)
	if err != nil {
		return nil, err
	}
	user := &app{path: file, data: data, lifetime: context.Background(), hosted: true}
	mux := user.routes()
	h.accounts[name] = mux
	return mux, nil
}

func desktopOnly(path string) bool {
	switch path {
	case "/api/fullscreen", "/api/update", "/api/exit", "/api/open-link", "/api/launch-game", "/api/launchers":
		return true
	default:
		return false
	}
}

func accountHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' https: data:; style-src 'self'; script-src 'self'; object-src 'none'; frame-src https://www.youtube-nocookie.com; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
}

func (h *accountHost) serve(w http.ResponseWriter, r *http.Request) {
	accountHeaders(w)
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		accountWeb.ServeHTTP(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		origin := r.Header.Get("Origin")
		local := r.Host == accountPort || strings.HasPrefix(r.Host, "localhost:")
		bearer := strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ")
		content := strings.HasPrefix(r.Header.Get("Content-Type"), "application/json")
		if !(origin == "https://"+r.Host || (local && origin == "http://"+r.Host) || (origin == "" && (bearer || content))) {
			fail(w, 403, "Die Anfrage stammt nicht von der Mediathek.")
			return
		}
	}
	switch {
	case r.URL.Path == "/api/account" && r.Method == http.MethodGet:
		name, _ := h.identify(r)
		reply(w, 200, accountStatus{Platform: "web", Name: name})
		return
	case r.URL.Path == "/api/account/session" && r.Method == http.MethodPost:
		h.authenticate(w, r)
		return
	case r.URL.Path == "/api/account/logout" && r.Method == http.MethodPost:
		h.logout(w, r)
		return
	case desktopOnly(r.URL.Path):
		http.NotFound(w, r)
		return
	}
	name, _ := h.identify(r)
	if name == "" {
		fail(w, http.StatusUnauthorized, "Melde dich zuerst an.")
		return
	}
	handler, err := h.account(name)
	if err != nil {
		fail(w, 500, "Die Sammlung konnte nicht geöffnet werden.")
		return
	}
	handler.ServeHTTP(w, r)
}

func (a *app) importLocalLibrary(w http.ResponseWriter, r *http.Request) {
	if !a.hosted {
		http.NotFound(w, r)
		return
	}
	var incoming library
	if !decodeLimited(w, r, &incoming, 32*1024*1024) {
		return
	}
	if len(incoming.Entries) > 25000 || len(incoming.Lists) > 500 || len(incoming.Goals) > 301 {
		fail(w, 400, "Die lokale Sammlung ist zu groß.")
		return
	}
	seen := map[string]bool{}
	for _, item := range incoming.Entries {
		if !valid(item) || seen[item.ID] {
			fail(w, 400, "Die lokale Sammlung enthält ungültige Einträge.")
			return
		}
		seen[item.ID] = true
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.data.Entries) > 0 || len(a.data.Lists) > 0 || len(a.data.Goals) > 0 {
		fail(w, http.StatusConflict, "Das Konto enthält bereits Daten. Die lokale Sammlung wurde nicht verändert.")
		return
	}
	next := a.data
	next.Entries = incoming.Entries
	if next.Entries == nil {
		next.Entries = []entry{}
	}
	next.Lists = incoming.Lists
	next.Goals = incoming.Goals
	a.commit(w, next)
}

type desktopAccount struct {
	Server string `json:"server"`
	Name   string `json:"name"`
	Token  string `json:"token"`
}
type accountStatus struct {
	Platform   string `json:"platform"`
	Server     string `json:"server,omitempty"`
	Name       string `json:"name,omitempty"`
	Invite     string `json:"invite,omitempty"`
	LocalCount int    `json:"localCount,omitempty"`
	HostActive bool   `json:"hostActive,omitempty"`
}

var accountHTTP = &http.Client{Timeout: 40 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}}

func validAccountServer(value string) string {
	address, err := url.Parse(strings.TrimSpace(value))
	if err != nil || address.Hostname() == "" || address.User != nil || address.RawQuery != "" || address.Fragment != "" || address.Opaque != "" || (address.Path != "" && address.Path != "/") {
		return ""
	}
	if address.Scheme != "https" && !(address.Scheme == "http" && address.Host == accountPort) {
		return ""
	}
	return address.Scheme + "://" + address.Host
}

func accountCall(ctx context.Context, connection desktopAccount, method, path string, body io.Reader, contentType string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, connection.Server+path, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if connection.Token != "" {
		request.Header.Set("Authorization", "Bearer "+connection.Token)
	}
	return accountHTTP.Do(request)
}

func readDesktopAccount(folder string) desktopAccount {
	var result desktopAccount
	data, err := os.ReadFile(filepath.Join(folder, "device-account.json"))
	if err != nil || json.Unmarshal(data, &result) != nil || validAccountServer(result.Server) == "" || !opaqueToken.MatchString(result.Token) {
		return desktopAccount{}
	}
	return result
}

func listenAccountHost(folder string) (*accountHost, *http.Server, error) {
	return listenAccountHostAt(folder, accountPort)
}

func listenAccountHostAt(folder, address string) (*accountHost, *http.Server, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, nil, err
	}
	host, err := openAccountHost(folder)
	if err != nil {
		_ = listener.Close()
		return nil, nil, err
	}
	server := &http.Server{Handler: http.HandlerFunc(host.serve), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 45 * time.Second, WriteTimeout: 50 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32768}
	go server.Serve(listener)
	return host, server, nil
}
