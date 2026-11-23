package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseNetflixHistory(t *testing.T) {
	csv := "\uFEFFTitel;Profilname;Typ;Zusatzvideotyp\r\n" +
		"Dark: Staffel 1: Geheimnisse;Max;;\nDark: Staffel 2: Anfänge;Max;;\n" +
		"\"Film: Ein Anfang\";Max;Film;\nFilmtrailer;Max;;Trailer\nDark;Alex;Serie;\n"
	got, err := parseHistory(csv, "csv")
	want := []historyGroup{
		{Profile: "Max", Titles: []historyTitle{{Title: "Dark", Kind: "series"}, {Title: "Film: Ein Anfang", Kind: "movie"}}},
		{Profile: "Alex", Titles: []historyTitle{{Title: "Dark", Kind: "series"}}},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("parseHistory = %#v, %v; want %#v", got, err, want)
	}
	if _, err := parseHistory("Titel,Datum\n\"Unclosed", "csv"); err == nil {
		t.Fatal("accepted a malformed CSV")
	}
}

func TestImportPreservesCollection(t *testing.T) {
	input := []entry{
		{ID: "existing", Kind: "movie", Title: "Der Film", Status: "want", Ratings: [3]int{8, 9, 10}},
		{ID: "series", Kind: "series", Title: "Dark", Status: "seen", Progress: 9, Genres: []string{"Sci-Fi"}},
		{ID: "book", Kind: "book", Title: "Dark", Status: "want"},
	}
	items := []historyTitle{{Title: "Der  Film", Kind: "movie"}, {Title: "dark", Kind: "series"}, {Title: "Television", Kind: "series"}}
	result := mergeHistory(input, items)
	if result.Added != 1 || result.Updated != 1 || result.Skipped != 1 || len(result.Entries) != 4 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if input[0].Status != "want" || result.Entries[0].Status != "seen" || result.Entries[0].Ratings != input[0].Ratings ||
		!reflect.DeepEqual(result.Entries[1:3], input[1:3]) || result.Entries[3].Status != "started" {
		t.Fatalf("import overwrote existing metadata or misclassified items: %+v", result.Entries)
	}
	repeat := mergeHistory(result.Entries, items)
	if repeat.Added != 0 || repeat.Updated != 0 || repeat.Skipped != 3 {
		t.Fatalf("import is not repeatable: %+v", repeat)
	}
}

func TestUpdateCheckRefusesModifiedInstaller(t *testing.T) {
	path := filepath.Join(t.TempDir(), "download.exe")
	if err := os.WriteFile(path, []byte("modified"), 0600); err != nil {
		t.Fatal(err)
	}
	check := updateCheck{status: updateStatus{State: "ready"}, installer: path, asset: releaseAsset{Size: 8}}
	if err := check.apply(); err == nil {
		t.Fatal("modified installer accepted")
	}
}
