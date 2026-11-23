package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSyncMergesDistinctFieldsAndKeepsConflictingDraft(t *testing.T) {
	a := &app{hosted: true, path: filepath.Join(t.TempDir(), "collection.json")}
	a.data.Entries = []entry{{ID: "manual:title", Kind: "movie", Title: "Original", Status: "want", Revision: "first"}}
	base := a.data.Entries[0]
	handler := a.routes()
	apply := func(ops ...syncOperation) syncResult {
		t.Helper()
		jsonBody, _ := json.Marshal(map[string][]syncOperation{"operations": ops})
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest("POST", "/api/sync", bytes.NewReader(jsonBody)))
		var result syncResult
		if recorder.Code != 200 || json.Unmarshal(recorder.Body.Bytes(), &result) != nil {
			t.Fatalf("sync: %d %s", recorder.Code, recorder.Body.String())
		}
		return result
	}
	desktop := base
	desktop.Title = "Vom PC"
	phone := base
	phone.Notes = "Vom iPhone"
	first := syncOperation{ID: "desktop-operation-1", ItemID: base.ID, Action: "edit", Base: &base, Entry: &desktop, Fields: []string{"title"}}
	second := syncOperation{ID: "iphone-operation-01", ItemID: base.ID, Action: "edit", Base: &base, Entry: &phone, Fields: []string{"notes"}}
	result := apply(first, second)
	if len(result.Accepted) != 2 || len(result.Conflicts) != 0 || result.Entries[0].Title != "Vom PC" || result.Entries[0].Notes != "Vom iPhone" {
		t.Fatalf("disjoint field merge: %+v", result)
	}
	retry := apply(second)
	if len(retry.Accepted) != 1 || retry.Entries[0].Revision != result.Entries[0].Revision {
		t.Fatalf("retry changed entry: %+v", retry)
	}
	phone.Title = "Anderer Titel"
	conflict := apply(syncOperation{ID: "iphone-operation-02", ItemID: base.ID, Action: "edit", Base: &base, Entry: &phone, Fields: []string{"title"}})
	if len(conflict.Accepted) != 0 || len(conflict.Conflicts) != 1 || conflict.Conflicts[0].Current.Title != "Vom PC" {
		t.Fatalf("same field conflict: %+v", conflict)
	}
}

func TestWindowsDraftSurvivesOfflineAndSynchronizes(t *testing.T) {
	var offline atomic.Bool
	serverApp := &app{hosted: true, path: filepath.Join(t.TempDir(), "server.json"), data: library{Entries: []entry{}}}
	routes := serverApp.routes()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if offline.Load() {
			w.WriteHeader(503)
			return
		}
		routes.ServeHTTP(w, r)
	}))
	defer server.Close()
	folder := t.TempDir()
	desktop := &app{path: filepath.Join(folder, "sammlung.json"), remote: desktopAccount{Name: "test", Server: server.URL, Token: "test"}}
	call := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		desktop.withAccountProxy(desktop.routes()).ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		if rec.Code != 200 {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
		}
		return rec
	}
	call("GET", "/api/collection", "")
	offline.Store(true)
	call("PUT", "/api/entries", `{"id":"manual:offline","kind":"book","title":"Unterwegs","status":"want"}`)
	if result := call("GET", "/api/collection", ""); !strings.Contains(result.Body.String(), "Unterwegs") {
		t.Fatalf("offline title missing: %s", result.Body.String())
	}
	if len(serverApp.data.Entries) != 0 {
		t.Fatal("test server was supposed to be unavailable")
	}
	desktop = &app{path: desktop.path, remote: desktop.remote}
	offline.Store(false)
	call("GET", "/api/collection", "")
	if result := call("GET", "/api/device-sync", ""); !strings.Contains(result.Body.String(), `"pending":0`) {
		t.Fatalf("draft was not accepted: %s", result.Body.String())
	}
	if len(serverApp.data.Entries) != 1 || serverApp.data.Entries[0].Title != "Unterwegs" {
		t.Fatalf("server collection: %+v", serverApp.data.Entries)
	}
}
