package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestCollectionMergeUndoPreservesEntries(t *testing.T) {
	first := entry{ID: "movie:imdb:tt101", Kind: "movie", Title: "Eins", Status: "want", Notes: "Erste Notiz", Lists: []string{"Abend"}}
	second := entry{ID: "manual:second", Kind: "movie", Title: "One", Status: "seen", Notes: "Zweite Notiz", Lists: []string{"Wochenende"}, Ratings: [3]int{8, 9, 10}, CompletedYear: 2026}
	app := &app{path: filepath.Join(t.TempDir(), "sammlung.json"), data: library{Entries: []entry{first, second}, Lists: []string{"Abend", "Wochenende"}}}
	call := func(method, route string, body any) *httptest.ResponseRecorder {
		r := httptest.NewRecorder()
		encoded, _ := json.Marshal(body)
		app.routes().ServeHTTP(r, httptest.NewRequest(method, route, bytes.NewReader(encoded)))
		return r
	}
	merged := call("POST", "/api/collection/merge", map[string]string{"keep": first.ID, "remove": second.ID, "ratings": second.ID, "progress": first.ID, "status": second.ID})
	if merged.Code != http.StatusOK {
		t.Fatalf("merge: %s", merged.Body.String())
	}
	got := app.data.Entries[0]
	if len(app.data.Entries) != 1 || got.Status != "seen" || got.Ratings != second.Ratings || len(got.Lists) != 2 || got.Notes != "Erste Notiz\n\nZweite Notiz" || got.CompletedYear != 2026 {
		t.Fatalf("merged data: %+v", got)
	}
	reverted := call("POST", "/api/collection/undo", nil)
	if reverted.Code != http.StatusOK || len(app.data.Entries) != 2 ||
		!equalEntry(app.data.Entries[0], first) || !equalEntry(app.data.Entries[1], second) {
		t.Fatalf("undo: %s", reverted.Body.String())
	}
}

func TestCatalogEnrichmentWorksWithImportUndo(t *testing.T) {
	result := mergeHistory(nil, []historyTitle{{Kind: "movie", Title: "Film"}})
	item := result.Entries[0]
	item.CatalogID = "movie:imdb:tt100"
	item.Genres = []string{"Drama"}
	item.Cover = "https://example.org/cover"
	preview, remaining := planHistoryUndo(library{Entries: []entry{item}, LastHistoryImport: result.changes})
	if preview.Removed != 1 || len(remaining) != 0 {
		t.Fatalf("enriched title prevented undo: %+v", preview)
	}
	item.Notes = "Eigene Notiz"
	preview, remaining = planHistoryUndo(library{Entries: []entry{item}, LastHistoryImport: result.changes})
	if preview.Preserved != 1 || len(remaining) != 1 {
		t.Fatalf("undo removed personal notes: %+v", preview)
	}
}
