package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestAccountIsolationImportAndRevisions(t *testing.T) {
	host, err := openAccountHost(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(host.serve))
	defer server.Close()
	call := func(method, path, token string, payload any) *http.Response {
		t.Helper()
		body, _ := json.Marshal(payload)
		r, _ := http.NewRequest(method, server.URL+path, bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		response, err := server.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { response.Body.Close() })
		return response
	}
	register := func(name string) string {
		t.Helper()
		response := call(http.MethodPost, "/api/account/session", "", accountRequest{Action: "register", Name: name, Password: "longer password", Invite: host.invite, Native: true})
		var current desktopAccount
		if response.StatusCode != 200 || json.NewDecoder(response.Body).Decode(&current) != nil || !opaqueToken.MatchString(current.Token) {
			t.Fatalf("registration failed: %d", response.StatusCode)
		}
		return current.Token
	}
	anonymous := call(http.MethodGet, "/api/entries", "", nil)
	if anonymous.StatusCode != 401 {
		t.Fatalf("anonymous access: %d", anonymous.StatusCode)
	}
	anna, bob := register("anna"), register("bob")
	imported := library{Entries: []entry{{ID: "manual:one", Kind: "movie", Status: "want", Title: "Ein Titel"}}, Settings: settings{SteamKey: "do not transfer", TMDB: "do not transfer"}}
	if code := call(http.MethodPut, "/api/account/import", anna, imported).StatusCode; code != 200 {
		t.Fatalf("import: %d", code)
	}
	if code := call(http.MethodPut, "/api/account/import", anna, imported).StatusCode; code != 409 {
		t.Fatalf("existing import: %d", code)
	}
	list := func(token string) []entry {
		t.Helper()
		response := call(http.MethodGet, "/api/entries", token, nil)
		var items []entry
		if response.StatusCode != 200 || json.NewDecoder(response.Body).Decode(&items) != nil {
			t.Fatalf("collection: %d", response.StatusCode)
		}
		return items
	}
	if items := list(bob); len(items) != 0 {
		t.Fatalf("another account can see entries: %v", items)
	}
	entries := list(anna)
	if len(entries) != 1 || entries[0].Revision == "" {
		t.Fatalf("import: %v", entries)
	}
	first := entries[0]
	edit := first
	edit.Title = "Neue Version"
	if code := call(http.MethodPut, "/api/entries", anna, edit).StatusCode; code != 200 {
		t.Fatalf("edit: %d", code)
	}
	first.Title = "Veralteter Stand"
	if code := call(http.MethodPut, "/api/entries", anna, first).StatusCode; code != 409 {
		t.Fatalf("stale write: %d", code)
	}
	if code := call(http.MethodDelete, "/api/entries", anna, first).StatusCode; code != 409 {
		t.Fatalf("stale delete: %d", code)
	}
	if title := list(anna)[0].Title; title != "Neue Version" {
		t.Fatalf("stale write overwrote: %s", title)
	}
	data, err := readLibrary(filepath.Join(host.folder, "collection-anna.json"))
	if err != nil || data.Settings.SteamKey != "" || data.Settings.TMDB != "" {
		t.Fatalf("imported settings: %+v %v", data.Settings, err)
	}
	if code := call(http.MethodPost, "/api/account/logout", anna, nil).StatusCode; code != 204 {
		t.Fatalf("logout: %d", code)
	}
	if code := call(http.MethodGet, "/api/entries", anna, nil).StatusCode; code != 401 {
		t.Fatalf("revoked token: %d", code)
	}
}

func TestAccountRejectsCrossOriginAndWindowsActions(t *testing.T) {
	host, err := openAccountHost(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "https://mediathek.example/api/account/session", strings.NewReader("{}"))
	req.Header.Set("Origin", "https://elsewhere.example")
	req.Header.Set("Content-Type", "application/json")
	out := httptest.NewRecorder()
	host.serve(out, req)
	if out.Code != 403 {
		t.Fatalf("cross-origin JSON: %d", out.Code)
	}
	out = httptest.NewRecorder()
	host.serve(out, httptest.NewRequest("GET", "https://mediathek.example/api/update", nil))
	if out.Code != 404 {
		t.Fatalf("native operation exposed: %d", out.Code)
	}
	if validAccountServer("http://mediathek.example") != "" || validAccountServer("https://user@mediathek.example") != "" {
		t.Fatal("insecure server accepted")
	}
	req = httptest.NewRequest("GET", "https://mediathek.example/mediathek.webmanifest", nil)
	out = httptest.NewRecorder()
	host.serve(out, req)
	payload, _ := io.ReadAll(out.Result().Body)
	if out.Code != 200 || !strings.Contains(string(payload), `"display": "standalone"`) {
		t.Fatalf("manifest: %d %s", out.Code, payload)
	}
}
