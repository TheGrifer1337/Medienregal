package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"
)

type collectionUndo struct {
	Before []entry `json:"before,omitempty"`
	After  []entry `json:"after,omitempty"`
	Label  string  `json:"label"`
}

type collectionState struct {
	Entries []entry  `json:"entries"`
	Lists   []string `json:"lists"`
	Undo    string   `json:"undo"`
}

func (a *app) collectionState() collectionState {
	data := collectionState{Entries: a.data.Entries, Lists: a.data.Lists}
	if a.data.Undo != nil {
		data.Undo = a.data.Undo.Label
	}
	return data
}

func (a *app) collection(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	reply(w, 200, a.collectionState())
}

func (a *app) saveCollection(w http.ResponseWriter, data library) {
	if err := writeLibrary(a.path, data); err != nil {
		fail(w, 500, "Die Sammlung konnte nicht gespeichert werden.")
		return
	}
	a.data = data
	reply(w, 200, a.collectionState())
}

func knownLists(names []string) map[string]string {
	result := make(map[string]string, len(names))
	for _, name := range names {
		result[historyKey(name)] = name
	}
	return result
}

func canonicalLists(values, names []string) []string {
	pool := knownLists(names)
	var result []string
	for _, value := range values {
		key := historyKey(value)
		if name := pool[key]; name != "" {
			result = append(result, name)
			delete(pool, key)
		}
	}
	return result
}

func (a *app) customList(w http.ResponseWriter, r *http.Request) {
	var request struct{ Name string }
	if !decode(w, r, &request) {
		return
	}
	name := strings.Join(strings.Fields(request.Name), " ")
	if name == "" || len(name) > 100 {
		fail(w, 400, "Gib einen Listennamen mit höchstens 100 Zeichen ein.")
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if knownLists(a.data.Lists)[historyKey(name)] != "" {
		fail(w, 409, "Es gibt bereits eine Liste mit diesem Namen.")
		return
	}
	next := a.data
	next.Lists = append(append([]string{}, next.Lists...), name)
	sort.Strings(next.Lists)
	a.saveCollection(w, next)
}

func (a *app) collectionView(w http.ResponseWriter, r *http.Request) {
	var request struct{ View string }
	if !decode(w, r, &request) {
		return
	}
	switch request.View {
	case "list", "small", "large":
	default:
		fail(w, 400, "Unbekannte Ansicht.")
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	next := a.data
	next.Settings.View = request.View
	a.commit(w, next)
}

func findEntry(entries []entry, id string) *entry {
	for _, item := range entries {
		if item.ID == id {
			return &item
		}
	}
	return nil
}

func equalEntry(a, b entry) bool {
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return bytes.Equal(left, right)
}

func replaceEntries(entries, before, after []entry) []entry {
	replaced := map[string]bool{}
	for _, item := range before {
		replaced[item.ID] = true
	}
	for _, item := range after {
		replaced[item.ID] = true
	}
	next := make([]entry, 0, len(entries)+len(after))
	for _, item := range entries {
		if !replaced[item.ID] {
			next = append(next, item)
		}
	}
	return append(next, after...)
}

func completed(item entry) bool {
	return item.Status == "seen" || item.Status == "finished" || (item.Kind == "book" && item.Status == "read")
}

func setCompletion(item *entry, old *entry) {
	item.CompletedYear = 0
	if !completed(*item) {
		return
	}
	if old != nil && completed(*old) {
		item.CompletedYear = old.CompletedYear
		return
	}
	item.CompletedYear = time.Now().Year()
}

func (a *app) undoCollection(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	undo := a.data.Undo
	if undo == nil {
		fail(w, 409, "Es gibt keine Änderung zum Rückgängigmachen.")
		return
	}
	for _, target := range undo.After {
		current := findEntry(a.data.Entries, target.ID)
		if current == nil || !equalEntry(*current, target) {
			fail(w, 409, "Die betroffenen Titel wurden inzwischen geändert.")
			return
		}
	}
	for _, target := range undo.Before {
		if findEntry(undo.After, target.ID) == nil && findEntry(a.data.Entries, target.ID) != nil {
			fail(w, 409, "Ein betroffener Titel wurde inzwischen erneut hinzugefügt.")
			return
		}
	}
	next := a.data
	next.Entries = replaceEntries(next.Entries, undo.After, undo.Before)
	next.Undo = nil
	a.saveCollection(w, next)
}

func (a *app) mergeCollection(w http.ResponseWriter, r *http.Request) {
	var request struct{ Keep, Remove, Ratings, Progress, Status string }
	if !decode(w, r, &request) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	left, right := findEntry(a.data.Entries, request.Keep), findEntry(a.data.Entries, request.Remove)
	if left == nil || right == nil || left.ID == right.ID || left.Kind != right.Kind {
		fail(w, 400, "Wähle zwei unterschiedliche Titel aus derselben Kategorie.")
		return
	}
	choice := func(id string) *entry {
		if id == left.ID {
			return left
		}
		if id == right.ID {
			return right
		}
		return nil
	}
	rating, progress, status := choice(request.Ratings), choice(request.Progress), choice(request.Status)
	if rating == nil || progress == nil || status == nil {
		fail(w, 400, "Wähle aus, welche Angaben übernommen werden.")
		return
	}
	item := *left
	item.Favorite = left.Favorite || right.Favorite
	if item.CustomCover == "" {
		item.CustomCover = right.CustomCover
	}
	item.Ratings = rating.Ratings
	item.Progress, item.Total = progress.Progress, progress.Total
	item.Status, item.CompletedYear = status.Status, status.CompletedYear
	if item.Cover == "" {
		item.Cover = right.Cover
	}
	if item.Subtitle == "" {
		item.Subtitle = right.Subtitle
	}
	if item.Description == "" {
		item.Description = right.Description
	}
	if item.CatalogID == "" && strings.HasPrefix(item.ID, "manual:") {
		item.CatalogID = right.CatalogID
		if item.CatalogID == "" && !strings.HasPrefix(right.ID, "manual:") {
			item.CatalogID = right.ID
		}
	}
	item.Lists = canonicalLists(append(append([]string{}, left.Lists...), right.Lists...), a.data.Lists)
	item.Genres = mergeLabels(left.Genres, right.Genres)
	item.Aliases = mergeLabels(left.Aliases, append([]string{right.Title}, right.Aliases...))
	if right.Notes != "" && item.Notes != right.Notes {
		item.Notes = strings.TrimSpace(strings.Join([]string{item.Notes, right.Notes}, "\n\n"))
	}
	item.ImportEdited = item.Source == "Streamingverlauf"
	if !valid(item) {
		fail(w, 400, "Die zusammengeführten Angaben sind zu lang oder ungültig.")
		return
	}
	next := a.data
	before := []entry{*left, *right}
	next.Entries = replaceEntries(next.Entries, before, []entry{item})
	next.Undo = &collectionUndo{Before: before, After: []entry{item}, Label: "Doppelte Titel zusammengeführt"}
	a.saveCollection(w, next)
}

func mergeLabels(a, b []string) []string {
	var result []string
	seen := map[string]bool{}
	for _, list := range [][]string{a, b} {
		for _, name := range list {
			key := historyKey(name)
			if key != "" && !seen[key] {
				result = append(result, name)
				seen[key] = true
			}
		}
	}
	return result
}

func (a *app) enrichEntry(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ID    string
		Match entry
	}
	if !decode(w, r, &request) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	old := findEntry(a.data.Entries, request.ID)
	match := request.Match
	if old == nil || old.Source != "Streamingverlauf" || old.Kind != match.Kind || !screenCatalogID(match.ID, match.Kind) {
		fail(w, 400, "Der Katalogtreffer passt nicht zum Importtitel.")
		return
	}
	item := *old
	item.CatalogID = match.ID
	item.Aliases = mergeLabels(item.Aliases, append([]string{match.Title}, match.Aliases...))
	if item.Cover == "" {
		item.Cover = match.Cover
	}
	if item.Subtitle == "" {
		item.Subtitle = match.Subtitle
	}
	if item.Description == "" {
		item.Description = match.Description
	}
	item.Genres = mergeLabels(item.Genres, match.Genres)
	if item.Total == 0 && match.Total >= item.Progress {
		item.Total = match.Total
	}
	if !valid(item) {
		fail(w, 400, "Die Katalogangaben konnten nicht übernommen werden.")
		return
	}
	next := a.data
	next.Entries = replaceEntries(next.Entries, []entry{*old}, []entry{item})
	a.saveCollection(w, next)
}

func screenCatalogID(id, kind string) bool {
	parts := strings.Split(id, ":")
	return (kind == "movie" || kind == "series") && len(parts) == 3 && parts[0] == kind &&
		((parts[1] == "imdb" && imdbID.MatchString(parts[2])) || (parts[1] == "tmdb" && numericID.MatchString(parts[2])))
}

func (a *app) favoriteEntry(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ID       string
		Favorite bool
	}
	if !decode(w, r, &request) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	old := findEntry(a.data.Entries, request.ID)
	if old == nil {
		fail(w, 404, "Der Titel ist nicht mehr in deiner Sammlung.")
		return
	}
	if old.Favorite == request.Favorite {
		reply(w, 200, a.collectionState())
		return
	}
	item := *old
	item.Favorite = request.Favorite
	if item.Source == "Streamingverlauf" {
		item.ImportEdited = true
	}
	next := a.data
	next.Entries = replaceEntries(next.Entries, []entry{*old}, []entry{item})
	next.Undo = &collectionUndo{Before: []entry{*old}, After: []entry{item}, Label: "Favorit geändert"}
	a.saveCollection(w, next)
}
