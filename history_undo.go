package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
)

type historyChange struct {
	ID             string `json:"id"`
	PreviousStatus string `json:"previousStatus,omitempty"`
}

type undoPreview struct {
	Mode      string   `json:"mode"`
	Removed   int      `json:"removed"`
	Restored  int      `json:"restored"`
	Preserved int      `json:"preserved"`
	Examples  []string `json:"examples"`
	Token     string   `json:"token"`
}

func historyStatus(kind string) string {
	if kind == "series" {
		return "started"
	}
	return "seen"
}

func historyID(item historyTitle) string {
	return fmt.Sprintf("manual:history-%x", sha256.Sum256([]byte(historyTitleKey(item))))
}

func uneditedHistoryTitle(item entry) bool {
	return !item.ImportEdited && !item.Favorite && item.CustomCover == "" && item.Status == historyStatus(item.Kind) &&
		item.ID == historyID(historyTitle{Title: item.Title, Kind: item.Kind}) &&
		(item.CatalogID != "" || (item.Subtitle == "" && item.Cover == "" && len(item.Genres) == 0 && item.Total == 0)) &&
		item.Progress == 0 && item.Notes == "" && len(item.Lists) == 0 && item.Ratings == [3]int{}
}

func planHistoryUndo(data library) (undoPreview, []entry) {
	preview := undoPreview{Mode: "older", Examples: []string{}}
	last := make(map[string]historyChange, len(data.LastHistoryImport))
	if len(data.LastHistoryImport) != 0 {
		preview.Mode = "last"
		for _, change := range data.LastHistoryImport {
			last[change.ID] = change
		}
	}

	entries := make([]entry, 0, len(data.Entries))
	var changes []historyChange
	for _, item := range data.Entries {
		change, affected := last[item.ID]
		if preview.Mode == "older" {
			affected = item.Source == "Streamingverlauf"
			change.ID = item.ID
		}
		if !affected {
			entries = append(entries, item)
			continue
		}

		if change.PreviousStatus == "" && uneditedHistoryTitle(item) {
			preview.Removed++
		} else if change.PreviousStatus == "want" && item.Status == historyStatus(item.Kind) {
			item.Status = change.PreviousStatus
			item.CompletedYear = 0
			entries = append(entries, item)
			preview.Restored++
		} else {
			preview.Preserved++
			entries = append(entries, item)
			continue
		}
		changes = append(changes, change)
		if len(preview.Examples) < 12 {
			preview.Examples = append(preview.Examples, item.Title)
		}
	}
	encoded, _ := json.Marshal(changes)
	preview.Token = fmt.Sprintf("%x", sha256.Sum256(append([]byte(preview.Mode), encoded...)))
	return preview, entries
}

func (a *app) historyUndo(w http.ResponseWriter, r *http.Request) {
	var request struct{ Token string }
	if r.Method == http.MethodPost && !decode(w, r, &request) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	preview, entries := planHistoryUndo(a.data)
	if r.Method == http.MethodGet {
		reply(w, http.StatusOK, preview)
		return
	}
	if request.Token != preview.Token {
		fail(w, http.StatusConflict, "Die Sammlung hat sich geändert. Öffne die Vorschau erneut.")
		return
	}
	if preview.Removed+preview.Restored == 0 {
		fail(w, http.StatusConflict, "Es gibt keine passenden Einträge zum Rückgängigmachen.")
		return
	}
	next := a.data
	next.Entries = entries
	next.LastHistoryImport = nil
	if err := writeLibrary(a.path, next); err != nil {
		fail(w, http.StatusInternalServerError, "Die Sammlung konnte nicht gespeichert werden.")
		return
	}
	a.data = next
	reply(w, http.StatusOK, struct {
		undoPreview
		Entries []entry `json:"entries"`
	}{preview, entries})
}
