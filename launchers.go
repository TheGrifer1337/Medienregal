package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

type launcherLocations struct {
	steam, epic, xbox, packages []string
}

var vdfValue = regexp.MustCompile(`(?m)"((?:\\.|[^"\\])*)"\s+"((?:\\.|[^"\\])*)"`)
var vdfUnescape = strings.NewReplacer(`\\`, `\`, `\"`, `"`)

func readLauncherFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 4*1024*1024 {
		return nil, fmt.Errorf("invalid manifest: %s", path)
	}
	return io.ReadAll(io.LimitReader(file, 4*1024*1024))
}

func scanLaunchers(ctx context.Context, paths launcherLocations) []entry {
	items := map[string]entry{}
	add := func(item entry) {
		item.Title = strings.TrimSpace(item.Title)
		item.Kind, item.Status = "game", "library"
		if valid(item) {
			items[item.ID] = item
		}
	}
	steamLibraries := map[string]bool{}
	for _, root := range paths.steam {
		steamLibraries[root] = true
		for _, name := range []string{"steamapps", "config"} {
			raw, _ := readLauncherFile(filepath.Join(root, name, "libraryfolders.vdf"))
			for _, pair := range vdfValue.FindAllStringSubmatch(string(raw), -1) {
				path := vdfUnescape.Replace(pair[2])
				_, numeric := strconv.Atoi(pair[1])
				if (strings.EqualFold(pair[1], "path") || numeric == nil) && filepath.IsAbs(path) {
					steamLibraries[filepath.Clean(path)] = true
				}
			}
		}
	}
	for root := range steamLibraries {
		if ctx.Err() != nil {
			break
		}
		scanSteam(filepath.Join(root, "steamapps"), add)
	}
	for _, dir := range paths.epic {
		if ctx.Err() == nil {
			scanEpic(dir, add)
		}
	}
	for _, root := range paths.xbox {
		if ctx.Err() == nil {
			scanXbox(root, add)
		}
	}
	for _, root := range paths.packages {
		if ctx.Err() == nil {
			readXboxConfig(root, filepath.Base(root), add)
		}
	}
	result := make([]entry, 0, len(items))
	for _, item := range items {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if a, b := historyKey(result[i].Title), historyKey(result[j].Title); a != b {
			return a < b
		}
		return result[i].ID < result[j].ID
	})
	return result
}

func installedDirectory(path string) bool {
	stat, err := os.Stat(path)
	return err == nil && stat.IsDir()
}

func scanSteam(dir string, add func(entry)) {
	files, _ := os.ReadDir(dir)
	for _, f := range files {
		name := f.Name()
		if !strings.HasPrefix(name, "appmanifest_") || !strings.HasSuffix(name, ".acf") {
			continue
		}
		id := strings.TrimSuffix(strings.TrimPrefix(name, "appmanifest_"), ".acf")
		if !numericID.MatchString(id) || id == "228980" {
			continue
		}
		raw, err := readLauncherFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		fields := map[string]string{}
		for _, pair := range vdfValue.FindAllStringSubmatch(string(raw), -1) {
			key := strings.ToLower(pair[1])
			if _, exists := fields[key]; !exists {
				fields[key] = vdfUnescape.Replace(pair[2])
			}
		}
		folder := fields["installdir"]
		if fields["appid"] != id || folder == "" || folder == "." || folder == ".." || strings.ContainsAny(folder, `/\`) ||
			!installedDirectory(filepath.Join(dir, "common", folder)) {
			continue
		}
		if flags, err := strconv.Atoi(fields["stateflags"]); err == nil && flags&4 == 0 {
			continue
		}
		title := fields["name"]
		if strings.HasPrefix(strings.ToLower(title), "steam linux runtime") || strings.HasPrefix(strings.ToLower(title), "proton ") {
			continue
		}
		add(entry{ID: "game:steam:" + id, Title: title, Source: "Steam", Cover: "https://cdn.cloudflare.steamstatic.com/steam/apps/" + id + "/header.jpg"})
	}
}

func scanEpic(dir string, add func(entry)) {
	files, _ := os.ReadDir(dir)
	for _, f := range files {
		if !strings.EqualFold(filepath.Ext(f.Name()), ".item") {
			continue
		}
		raw, err := readLauncherFile(filepath.Join(dir, f.Name()))
		if err != nil {
			continue
		}
		var manifest struct {
			AppName          string
			MainGameAppName  string
			DisplayName      string
			InstallLocation  string
			LaunchExecutable string
			CatalogNamespace string
			CatalogItemID    string
			Incomplete       bool     `json:"bIsIncompleteInstall"`
			DLC              bool     `json:"bIsDLC"`
			Categories       []string `json:"AppCategories"`
		}
		if json.Unmarshal(bytes.TrimPrefix(raw, []byte("\ufeff")), &manifest) != nil || manifest.Incomplete || manifest.DLC || manifest.AppName == "" ||
			(manifest.MainGameAppName != "" && manifest.MainGameAppName != manifest.AppName) ||
			!filepath.IsAbs(manifest.InstallLocation) || !installedDirectory(manifest.InstallLocation) {
			continue
		}
		if strings.HasPrefix(strings.ToLower(manifest.AppName), "ue_") || strings.Contains(strings.ToLower(strings.Join(manifest.Categories, " ")), "engine") {
			continue
		}
		if manifest.LaunchExecutable == "" {
			continue
		}
		key := launcherID(manifest.CatalogNamespace + ":" + manifest.CatalogItemID + ":" + manifest.AppName)
		add(entry{ID: "game:epic:" + key, Title: manifest.DisplayName, Source: "Epic Games", launchData: epicLaunchURL(manifest.AppName, manifest.CatalogNamespace, manifest.CatalogItemID)})
	}
}

func launcherID(value string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.ToLower(value))))
}

func xboxRootPaths(drive string, raw []byte) []string {
	if len(raw) < 10 || string(raw[:4]) != "RGBX" {
		return nil
	}
	var paths []string
	var units []uint16
	for offset := 8; offset+1 < len(raw); offset += 2 {
		unit := binary.LittleEndian.Uint16(raw[offset:])
		if unit != 0 {
			units = append(units, unit)
			continue
		}
		if len(units) == 0 {
			continue
		}
		name := strings.Trim(string(utf16.Decode(units)), "\uFEFF/\\")
		units = nil
		name = filepath.Clean(strings.ReplaceAll(name, `\`, string(filepath.Separator)))
		if name != "" && name != "." && name != ".." && !strings.HasPrefix(name, ".."+string(filepath.Separator)) &&
			!strings.ContainsRune(name, ':') && !filepath.IsAbs(name) {
			paths = append(paths, filepath.Join(drive, name))
		}
	}
	return paths
}

func scanXbox(root string, add func(entry)) {
	dirs, _ := os.ReadDir(root)
	for _, dir := range dirs {
		if !dir.IsDir() {
			continue
		}
		game := filepath.Join(root, dir.Name())
		if !readXboxConfig(filepath.Join(game, "Content"), dir.Name(), add) {
			readXboxConfig(game, dir.Name(), add)
		}
	}
}

func readXboxConfig(root, folder string, add func(entry)) bool {
	raw, err := readLauncherFile(filepath.Join(root, "MicrosoftGame.config"))
	if err != nil {
		return false
	}
	var config struct {
		XMLName  xml.Name `xml:"Game"`
		Identity struct {
			Name string `xml:"Name,attr"`
		} `xml:"Identity"`
		Visuals struct {
			Display string `xml:"DefaultDisplayName,attr"`
		} `xml:"ShellVisuals"`
		Executable []struct {
			Display string `xml:"OverrideDisplayName,attr"`
		} `xml:"ExecutableList>Executable"`
		StoreID string `xml:"StoreId"`
	}
	if xml.Unmarshal(raw, &config) != nil {
		return false
	}
	title := config.Visuals.Display
	if title == "" && len(config.Executable) > 0 {
		title = config.Executable[0].Display
	}
	if title == "" || strings.HasPrefix(strings.ToLower(title), "ms-resource:") {
		title = folder
	}
	key := strings.TrimSpace(config.StoreID)
	if key == "" {
		key = config.Identity.Name
	}
	if key == "" {
		key = folder
	}
	add(entry{ID: "game:xbox:" + launcherID(key), Title: title, Source: "Xbox", launchData: config.Identity.Name})
	return true
}

func knownLauncherGames(entries []entry) map[string]bool {
	existing := make(map[string]bool, len(entries)*2)
	for _, item := range entries {
		if item.Kind != "game" {
			continue
		}
		existing[item.ID] = true
		existing[item.CatalogID] = true
		existing["title:"+historyKey(item.Title)] = true
	}
	return existing
}

func launcherDuplicate(known map[string]bool, item entry) bool {
	return known[item.ID] || known["title:"+historyKey(item.Title)]
}

func (a *app) previewLaunchers(w http.ResponseWriter, r *http.Request) {
	items := scanLaunchers(r.Context(), localLauncherLocations(r.Context()))
	if r.Context().Err() != nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	known := knownLauncherGames(a.data.Entries)
	a.launcherGames = make(map[string]entry, len(items))
	type candidate struct {
		entry
		Existing bool `json:"existing"`
	}
	result := make([]candidate, 0, len(items))
	for _, item := range items {
		a.launcherGames[item.ID] = item
		result = append(result, candidate{item, launcherDuplicate(known, item)})
	}
	reply(w, 200, result)
}

func (a *app) importLaunchers(w http.ResponseWriter, r *http.Request) {
	var request struct{ IDs []string }
	if !decodeLimited(w, r, &request, 512*1024) {
		return
	}
	if len(request.IDs) == 0 || len(request.IDs) > 4000 {
		fail(w, 400, "Wähle zwischen 1 und 4.000 Spielen zum Import aus.")
		return
	}
	a.mu.Lock()
	items := make([]entry, 0, len(request.IDs))
	for _, id := range request.IDs {
		item, ok := a.launcherGames[id]
		if !ok {
			a.mu.Unlock()
			fail(w, 409, "Die Auswahl ist veraltet. Scanne die Launcher erneut.")
			return
		}
		item.launchData = ""
		items = append(items, item)
	}
	a.mu.Unlock()
	a.saveLauncherSelection(w, items)
}

func (a *app) saveLauncherSelection(w http.ResponseWriter, items []entry) {
	a.mu.Lock()
	defer a.mu.Unlock()
	known := knownLauncherGames(a.data.Entries)
	var added []entry
	for _, item := range items {
		if launcherDuplicate(known, item) {
			continue
		}
		known[item.ID], known["title:"+historyKey(item.Title)] = true, true
		added = append(added, item)
	}
	next := a.data
	if len(added) > 0 {
		next.Entries = replaceEntries(next.Entries, nil, added)
		next.Undo = &collectionUndo{After: added, Label: "Launcher-Import"}
		if err := a.persist(next); err != nil {
			fail(w, 500, "Die Spiele konnten nicht gespeichert werden.")
			return
		}
	}
	reply(w, 200, struct {
		collectionState
		Added   int `json:"added"`
		Skipped int `json:"skipped"`
	}{a.collectionState(), len(added), len(items) - len(added)})
}
