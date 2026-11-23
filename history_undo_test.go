package main

import (
	"reflect"
	"testing"
)

func TestUndoHistoryFromOlderVersions(t *testing.T) {
	normal := entry{ID: "manual:123", Kind: "movie", Title: "On the list", Status: "seen"}
	result := mergeHistory([]entry{normal}, []historyTitle{
		{Title: "Old film", Kind: "movie"},
		{Title: "Edited film", Kind: "movie"},
		{Title: "Another film", Kind: "movie"},
	})
	result.Entries[2].Ratings = [3]int{9, 8, 10}
	result.Entries[3].Title = "Renamed"
	preview, remaining := planHistoryUndo(library{Entries: result.Entries})
	if preview.Mode != "older" || preview.Removed != 1 || preview.Preserved != 2 ||
		!reflect.DeepEqual(remaining, []entry{normal, result.Entries[2], result.Entries[3]}) {
		t.Fatalf("undo removed more than unchanged imports: %+v, %+v", preview, remaining)
	}
}

func TestUndoRestoresLatestImportAndKeepsOlderEntries(t *testing.T) {
	wanted := entry{ID: "movie:imdb:tt1", Kind: "movie", Title: "Want to see", Status: "want", Ratings: [3]int{9, 0, 0}}
	older := mergeHistory([]entry{wanted}, []historyTitle{{Title: "Older", Kind: "series"}}).Entries[1]
	current := mergeHistory([]entry{wanted, older}, []historyTitle{
		{Title: wanted.Title, Kind: wanted.Kind}, {Title: "Latest", Kind: "movie"},
	})
	preview, remaining := planHistoryUndo(library{Entries: current.Entries, LastHistoryImport: current.changes})
	if preview.Mode != "last" || preview.Restored != 1 || preview.Removed != 1 ||
		!reflect.DeepEqual(remaining, []entry{wanted, older}) {
		t.Fatalf("undo did not restore the original collection: %+v, %+v", preview, remaining)
	}
}
