package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestScanLaunchers(t *testing.T) {
	root := t.TempDir()
	mkdir := func(path string) string {
		t.Helper()
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		return path
	}
	write := func(path, body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	steam := mkdir(filepath.Join(root, "Steam", "steamapps"))
	secondary := filepath.Join(root, "Game Library")
	write(filepath.Join(steam, "libraryfolders.vdf"), `"libraryfolders" { "1" { "path" "`+filepath.ToSlash(secondary)+`" } }`)
	mkdir(filepath.Join(secondary, "steamapps", "common", "Aperture"))
	write(filepath.Join(secondary, "steamapps", "appmanifest_620.acf"), `"AppState" { "appid" "620" "name" "Portal 2" "StateFlags" "4" "installdir" "Aperture" }`)
	write(filepath.Join(secondary, "steamapps", "appmanifest_621.acf"), `"AppState" { "appid" "621" "name" "Downloading" "StateFlags" "2" "installdir" "Aperture" }`)

	epic := mkdir(filepath.Join(root, "EpicManifests"))
	install := mkdir(filepath.Join(root, "Epic", "Celeste"))
	base := map[string]string{"AppName": "Celeste", "DisplayName": "Celeste", "InstallLocation": install, "LaunchExecutable": "Celeste.exe"}
	manifest, _ := json.Marshal(base)
	write(filepath.Join(epic, "base.item"), "\ufeff"+string(manifest))
	base["AppName"], base["MainGameAppName"], base["DisplayName"] = "CelesteDLC", "Celeste", "Extra content"
	manifest, _ = json.Marshal(base)
	write(filepath.Join(epic, "dlc.item"), string(manifest))

	marker := []byte{'R', 'G', 'B', 'X', 1, 0, 0, 0}
	for _, unit := range utf16.Encode([]rune("\\Spieleordner\\Äxbox\x00..\\outside\x00")) {
		marker = binary.LittleEndian.AppendUint16(marker, unit)
	}
	xbox := xboxRootPaths(root, marker)
	if len(xbox) != 1 || xbox[0] != filepath.Join(root, "Spieleordner", "Äxbox") {
		t.Fatalf("unexpected Xbox game roots: %q", xbox)
	}
	content := mkdir(filepath.Join(xbox[0], "Named folder", "Content"))
	write(filepath.Join(content, "MicrosoftGame.config"), `<Game><Identity Name="Microsoft.Example"/><ShellVisuals DefaultDisplayName="Forza Horizon"/><StoreId>1234</StoreId></Game>`)
	xboxOptions := []string{filepath.Join(root, "XboxGames"), xbox[0]}
	items := scanLaunchers(context.Background(), launcherLocations{steam: []string{filepath.Dir(steam)}, epic: []string{epic}, xbox: xboxOptions})
	if len(items) != 3 {
		t.Fatalf("expected three installed games, got %+v", items)
	}
	for i, expected := range []struct{ title, source, id string }{
		{"Celeste", "Epic Games", "game:epic:" + launcherID("::Celeste")},
		{"Forza Horizon", "Xbox", "game:xbox:" + launcherID("1234")},
		{"Portal 2", "Steam", "game:steam:620"},
	} {
		item := items[i]
		if item.Title != expected.title || item.Source != expected.source || item.ID != expected.id || item.Kind != "game" || item.Status != "library" {
			t.Errorf("unexpected game: %+v", item)
		}
	}
}

func TestLauncherImportDeduplicatesAndCanBeUndone(t *testing.T) {
	existing := entry{ID: "manual:0", Kind: "game", Title: "Portal 2", Status: "played", Notes: "Meine Notizen"}
	steam := entry{ID: "game:steam:620", Kind: "game", Title: "Portal 2", Status: "library"}
	epic := entry{ID: "game:epic:celeste", Kind: "game", Title: "Celeste", Status: "library"}
	xbox := entry{ID: "game:xbox:celeste", Kind: "game", Title: "Celeste", Status: "library"}
	app := &app{
		path:          filepath.Join(t.TempDir(), "sammlung.json"),
		data:          library{Entries: []entry{existing}},
		launcherGames: map[string]entry{steam.ID: steam, epic.ID: epic, xbox.ID: xbox},
	}
	call := func(path, body string) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		app.routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
		return recorder
	}
	stale := call("/api/launchers", `{"ids":["game:epic:celeste","unknown"]}`)
	if stale.Code != http.StatusConflict || len(app.data.Entries) != 1 {
		t.Fatalf("stale import changed the library: %d %s", stale.Code, stale.Body.String())
	}
	selected := `{"ids":["game:steam:620","game:epic:celeste","game:xbox:celeste"]}`
	for i := 0; i < 2; i++ {
		result := call("/api/launchers", selected)
		var counts struct{ Added, Skipped int }
		if result.Code != http.StatusOK || json.Unmarshal(result.Body.Bytes(), &counts) != nil ||
			counts.Added != 1-i || counts.Skipped != 2+i || len(app.data.Entries) != 2 {
			t.Fatalf("import %d: %d %s", i+1, result.Code, result.Body.String())
		}
	}
	undo := call("/api/collection/undo", "")
	if undo.Code != http.StatusOK || len(app.data.Entries) != 1 || !equalEntry(app.data.Entries[0], existing) {
		t.Fatalf("undo changed the previous collection: %d %s", undo.Code, undo.Body.String())
	}
}
