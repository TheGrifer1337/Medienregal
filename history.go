package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"
)

const historyLimit = 4 * 1024 * 1024
const maxHistoryTitles = 10000

var episodeSuffix = regexp.MustCompile(`(?i):\s*(?:(?:staffel|season|saison|temporada|folge|episode)\s+[0-9]+|limited series|miniserie|miniseries)(?:\s*:.*)?\s*$`)
var numberedEpisode = regexp.MustCompile(`(?i)(?:\s*[:–-]\s*|\s+)s[0-9]{1,3}\s*e[0-9]{1,4}(?:\s*[:–-].*)?\s*$`)

type historyTitle struct {
	Title string `json:"title"`
	Kind  string `json:"kind"`
}

type historyGroup struct {
	Profile string         `json:"profile"`
	Titles  []historyTitle `json:"titles"`
}

type historyResult struct {
	Added   int     `json:"added"`
	Updated int     `json:"updated"`
	Skipped int     `json:"skipped"`
	Entries []entry `json:"entries"`
	changes []historyChange
}

func historyKey(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func historyTitleKey(item historyTitle) string {
	return item.Kind + ":" + historyKey(item.Title)
}

func cleanHistoryTitle(title, kind string) historyTitle {
	title = strings.Join(strings.Fields(title), " ")
	if kind != "movie" {
		for _, pattern := range []*regexp.Regexp{episodeSuffix, numberedEpisode} {
			if location := pattern.FindStringIndex(title); location != nil && location[0] > 0 {
				return historyTitle{Title: strings.TrimSpace(title[:location[0]]), Kind: "series"}
			}
		}
	}
	return historyTitle{Title: title, Kind: kind}
}

func historyKind(value string) string {
	switch historyKey(value) {
	case "film", "movie":
		return "movie"
	case "series", "serie", "show", "tv", "tv show", "episode", "folge":
		return "series"
	default:
		return ""
	}
}

func parseHistory(content, format string) ([]historyGroup, error) {
	if !utf8.ValidString(content) {
		return nil, errors.New("Die Datei muss UTF-8-kodiert sein.")
	}
	content = strings.TrimPrefix(content, "\uFEFF")
	groups := []historyGroup{}
	profiles := map[string]int{}
	type profileTitle struct{ profile, title string }
	seen := map[profileTitle]bool{}
	add := func(profile string, item historyTitle) error {
		if item.Title == "" {
			return nil
		}
		if len(item.Title) > 600 || len(profile) > 600 {
			return errors.New("Eine Zeile enthält einen zu langen Titel oder Profilnamen.")
		}
		key := profileTitle{profile, historyTitleKey(item)}
		if seen[key] {
			return nil
		}
		if len(seen) >= maxHistoryTitles {
			return errors.New("Die Datei enthält mehr als 10.000 unterschiedliche Einträge.")
		}
		seen[key] = true
		index, exists := profiles[profile]
		if !exists {
			index = len(groups)
			profiles[profile] = index
			groups = append(groups, historyGroup{Profile: profile})
		}
		groups[index].Titles = append(groups[index].Titles, item)
		return nil
	}
	if format == "text" {
		for _, title := range strings.Split(content, "\n") {
			if err := add("", cleanHistoryTitle(title, "")); err != nil {
				return nil, err
			}
		}
		return groups, nil
	}
	reader, columns := openHistoryCSV(content)
	if reader == nil {
		return nil, errors.New("Die CSV-Datei braucht eine Spalte „Title“ oder „Titel“. Verwende den Netflix-Titelverlauf oder eine eigene CSV-Datei.")
	}
	for {
		fields, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("Die CSV-Datei ist beschädigt (Zeile %d).", csvErrorLine(err))
		}
		value := func(column string) string {
			if index, exists := columns[column]; exists && index < len(fields) {
				return strings.TrimSpace(fields[index])
			}
			return ""
		}
		if value("extra") != "" {
			continue
		}
		title := cleanHistoryTitle(value("title"), historyKind(value("kind")))
		if err := add(value("profile"), title); err != nil {
			return nil, err
		}
	}
	return groups, nil
}

func openHistoryCSV(content string) (*csv.Reader, map[string]int) {
	for _, delimiter := range []rune{',', ';', '\t'} {
		reader := csv.NewReader(strings.NewReader(content))
		reader.Comma = delimiter
		reader.FieldsPerRecord = -1
		header, err := reader.Read()
		if err != nil {
			continue
		}
		columns := map[string]int{}
		for i, value := range header {
			switch historyKey(value) {
			case "title", "titel":
				columns["title"] = i
			case "profile name", "profile", "profil", "profilname", "name des profils":
				columns["profile"] = i
			case "supplemental video type", "zusatzvideotyp", "typ des zusatzvideos":
				columns["extra"] = i
			case "typ", "type", "kind", "medientyp":
				columns["kind"] = i
			}
		}
		if _, found := columns["title"]; found {
			return reader, columns
		}
	}
	return nil, nil
}

func csvErrorLine(err error) int {
	var parseError *csv.ParseError
	if errors.As(err, &parseError) {
		return parseError.Line
	}
	return 0
}

func (a *app) previewHistory(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	if format != "text" && format != "csv" {
		fail(w, 400, "Unbekanntes Importformat.")
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, historyLimit))
	if err != nil {
		fail(w, 400, "Die Datei konnte nicht gelesen werden. Maximal 4 MB sind möglich.")
		return
	}
	groups, err := parseHistory(string(data), format)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	if len(groups) == 0 {
		fail(w, 400, "Es wurden keine Titel zum Importieren gefunden.")
		return
	}
	reply(w, 200, groups)
}

func mergeHistory(existing []entry, titles []historyTitle) historyResult {
	result := historyResult{Entries: append(make([]entry, 0, len(existing)+len(titles)), existing...)}
	index := make(map[string][]int)
	ids := make(map[string]bool)
	for i, item := range existing {
		ids[item.ID] = true
		key := historyTitleKey(historyTitle{Title: item.Title, Kind: item.Kind})
		index[key] = append(index[key], i)
	}
	for _, item := range titles {
		key := historyTitleKey(item)
		status := historyStatus(item.Kind)
		matches := index[key]
		if len(matches) == 0 {
			id := historyID(item)
			// Renamed imports can already own the original ID.
			if ids[id] {
				result.Skipped++
				continue
			}
			ids[id] = true
			index[key] = []int{len(result.Entries)}
			added := entry{ID: id, Kind: item.Kind, Title: item.Title, Status: status, Source: "Streamingverlauf"}
			setCompletion(&added, nil)
			result.Entries = append(result.Entries, added)
			result.changes = append(result.changes, historyChange{ID: id})
			result.Added++
		} else if len(matches) == 1 && result.Entries[matches[0]].Status == "want" {
			result.changes = append(result.changes, historyChange{ID: result.Entries[matches[0]].ID, PreviousStatus: "want"})
			result.Entries[matches[0]].Status = status
			setCompletion(&result.Entries[matches[0]], nil)
			result.Updated++
		} else {
			result.Skipped++
		}
	}
	return result
}

func (a *app) importHistory(w http.ResponseWriter, r *http.Request) {
	var titles []historyTitle
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, historyLimit))
	if decoder.Decode(&titles) != nil || decoder.Decode(new(any)) != io.EOF || len(titles) == 0 || len(titles) > maxHistoryTitles {
		fail(w, 400, "Wähle gültige Titel für den Import aus.")
		return
	}
	for i, item := range titles {
		titles[i].Title = strings.Join(strings.Fields(item.Title), " ")
		if (item.Kind != "movie" && item.Kind != "series") || titles[i].Title == "" || len(titles[i].Title) > 600 {
			fail(w, 400, "Der Import enthält einen ungültigen Titel oder Medientyp.")
			return
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	result := mergeHistory(a.data.Entries, titles)
	if result.Added+result.Updated > 0 {
		next := a.data
		next.Entries = result.Entries
		next.LastHistoryImport = result.changes
		if err := writeLibrary(a.path, next); err != nil {
			fail(w, 500, "Die Auswahl konnte nicht gespeichert werden.")
			return
		}
		a.data = next
	}
	reply(w, 200, result)
}
