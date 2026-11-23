package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"regexp"
	"slices"
)

type syncOperation struct {
	ID     string   `json:"id"`
	ItemID string   `json:"itemId"`
	Action string   `json:"action"`
	Entry  *entry   `json:"entry,omitempty"`
	Base   *entry   `json:"base,omitempty"`
	Fields []string `json:"fields,omitempty"`
}
type syncConflict struct {
	ID      string `json:"id"`
	Current *entry `json:"current,omitempty"`
}
type syncResult struct {
	Undo      string         `json:"undo,omitempty"`
	Entries   []entry        `json:"entries"`
	Lists     []string       `json:"lists"`
	Accepted  []string       `json:"accepted"`
	Conflicts []syncConflict `json:"conflicts"`
}

var syncOperationID = regexp.MustCompile(`^[a-zA-Z0-9_-]{16,80}$`)
var syncEditableFields = []string{"title", "subtitle", "status", "progress", "total", "ratings", "favorite", "notes", "genres", "lists", "cover", "customCover", "loan", "series", "sequence", "trailer", "imdb", "catalogId", "source", "aliases", "description"}

func (a *app) syncCollection(w http.ResponseWriter, r *http.Request) {
	var request struct{ Operations []syncOperation }
	if !decodeLimited(w, r, &request, 8*1024*1024) {
		return
	}
	if len(request.Operations) > 20 {
		fail(w, 400, "Maximal 20 Änderungen pro Abgleich.")
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	next := a.data
	next.Entries = append([]entry{}, a.data.Entries...)
	next.SyncReceipts = slices.Clone(a.data.SyncReceipts)
	result := syncResult{Accepted: []string{}, Conflicts: []syncConflict{}}
	blocked := map[string]bool{}
	changed := false
	for _, op := range request.Operations {
		if !validSyncOperation(op) {
			fail(w, 400, "Eine Änderung ist ungültig.")
			return
		}
		if slices.Contains(next.SyncReceipts, op.ID) {
			result.Accepted = append(result.Accepted, op.ID)
			continue
		}
		current := findEntry(next.Entries, op.ItemID)
		if blocked[op.ItemID] {
			result.Conflicts = append(result.Conflicts, syncConflict{op.ID, current})
			continue
		}
		updated, applied := applySyncOperation(current, op, next.Lists)
		if !applied {
			blocked[op.ItemID] = true
			result.Conflicts = append(result.Conflicts, syncConflict{op.ID, current})
			continue
		}
		var before []entry
		if current != nil {
			before = []entry{*current}
		}
		if updated == nil && current != nil {
			next.Entries = replaceEntries(next.Entries, before, nil)
			next.Undo = &collectionUndo{Before: before, Label: "Titel entfernt"}
		} else if updated != nil && (current == nil || !equalEntry(*current, *updated)) {
			next.Entries = replaceEntries(next.Entries, before, []entry{*updated})
			next.Undo = &collectionUndo{Before: before, After: []entry{*updated}, Label: "Titel gespeichert"}
		}
		next.SyncReceipts = append(next.SyncReceipts, op.ID)
		result.Accepted = append(result.Accepted, op.ID)
		changed = true
	}
	if changed {
		if len(next.SyncReceipts) > 8192 {
			next.SyncReceipts = slices.Clone(next.SyncReceipts[len(next.SyncReceipts)-8192:])
		}
		if err := a.persist(next); err != nil {
			fail(w, 500, "Der Abgleich konnte nicht gespeichert werden.")
			return
		}
	}
	result.Entries = a.data.Entries
	result.Lists = a.data.Lists
	result.Undo = a.collectionState().Undo
	reply(w, 200, result)
}

func validSyncOperation(op syncOperation) bool {
	if !syncOperationID.MatchString(op.ID) || op.ItemID == "" || len(op.ItemID) > 200 || len(op.Fields) > 24 {
		return false
	}
	switch op.Action {
	case "add":
		return op.Entry != nil && op.Entry.ID == op.ItemID && valid(*op.Entry)
	case "edit":
		return op.Entry != nil && op.Base != nil && op.ItemID == op.Entry.ID && op.ItemID == op.Base.ID && op.Entry.Kind == op.Base.Kind && len(op.Fields) > 0 &&
			slices.IndexFunc(op.Fields, func(field string) bool { return !slices.Contains(syncEditableFields, field) }) < 0
	case "delete":
		return op.Base != nil && op.Base.ID == op.ItemID
	}
	return false
}

func syncFields(item *entry) map[string]json.RawMessage {
	fields := map[string]json.RawMessage{}
	raw, _ := json.Marshal(item)
	_ = json.Unmarshal(raw, &fields)
	return fields
}

func applySyncOperation(current *entry, op syncOperation, lists []string) (*entry, bool) {
	switch op.Action {
	case "add":
		if current != nil {
			return current, false
		}
		updated := *op.Entry
		updated.Revision = ""
		updated.Lists = canonicalLists(updated.Lists, lists)
		setCompletion(&updated, nil)
		return &updated, true
	case "delete":
		if current == nil {
			return nil, true
		}
		return nil, (op.Base.Revision != "" && op.Base.Revision == current.Revision) || (op.Base.Revision == "" && current.Revision == "" && len(syncChangedFields(op.Base, current)) == 0)
	case "edit":
		if current == nil || current.Kind != op.Entry.Kind {
			return nil, false
		}
		now, base, requested := syncFields(current), syncFields(op.Base), syncFields(op.Entry)
		for _, field := range op.Fields {
			if !bytes.Equal(now[field], base[field]) && !bytes.Equal(now[field], requested[field]) {
				return nil, false
			}
			if value, ok := requested[field]; ok {
				now[field] = value
			} else {
				delete(now, field)
			}
		}
		raw, err := json.Marshal(now)
		if err != nil {
			return nil, false
		}
		var updated entry
		if json.Unmarshal(raw, &updated) != nil || !valid(updated) {
			return nil, false
		}
		updated.Lists = canonicalLists(updated.Lists, lists)
		setCompletion(&updated, current)
		if current.Source == "Streamingverlauf" && !equalEntry(*current, updated) {
			updated.ImportEdited = true
		}
		return &updated, true
	}
	return nil, false
}

func syncChangedFields(base, updated *entry) []string {
	before, after := syncFields(base), syncFields(updated)
	fields := []string{}
	for _, key := range syncEditableFields {
		if !bytes.Equal(before[key], after[key]) {
			fields = append(fields, key)
		}
	}
	return fields
}
