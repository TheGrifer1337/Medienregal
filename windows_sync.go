package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"time"
)

type deviceSync struct {
	Snapshot    collectionState `json:"snapshot"`
	Preferences json.RawMessage `json:"preferences,omitempty"`
	Goals       json.RawMessage `json:"goals,omitempty"`
	Pending     []syncOperation `json:"pending,omitempty"`
	Conflicts   []syncConflict  `json:"conflicts,omitempty"`
}

func (d deviceSync) visible() collectionState {
	view := d.Snapshot
	view.Entries = append([]entry{}, view.Entries...)
	if len(d.Pending) > 0 {
		view.Undo = ""
	}
	for _, op := range d.Pending {
		current := findEntry(view.Entries, op.ItemID)
		var before []entry
		if current != nil {
			before = []entry{*current}
		}
		switch op.Action {
		case "add":
			view.Entries = replaceEntries(view.Entries, before, []entry{*op.Entry})
		case "edit":
			item := *op.Entry
			if current != nil {
				fields := syncFields(current)
				requested := syncFields(op.Entry)
				for _, field := range op.Fields {
					if value, ok := requested[field]; ok {
						fields[field] = value
					} else {
						delete(fields, field)
					}
				}
				if raw, err := json.Marshal(fields); err == nil {
					var merged entry
					if json.Unmarshal(raw, &merged) == nil {
						item = merged
					}
				}
			}
			view.Entries = replaceEntries(view.Entries, before, []entry{item})
		case "delete":
			view.Entries = replaceEntries(view.Entries, before, nil)
		}
	}
	return view
}

func (a *app) readDeviceSync(remote desktopAccount) (deviceSync, string, error) {
	path := filepath.Join(filepath.Dir(a.path), "sync-"+sessionHash(remote.Server+"\n"+remote.Name)+".json")
	var data deviceSync
	raw, err := os.ReadFile(path)
	if err == nil {
		err = json.Unmarshal(raw, &data)
	}
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	return data, path, err
}

func runDeviceSync(ctx context.Context, remote desktopAccount, data *deviceSync, path string) error {
	blocked := map[string]bool{}
	for _, conflict := range data.Conflicts {
		for _, op := range data.Pending {
			if op.ID == conflict.ID {
				blocked[op.ItemID] = true
			}
		}
	}
	waiting := []syncOperation{}
	for _, op := range data.Pending {
		if !blocked[op.ItemID] {
			waiting = append(waiting, op)
		}
		if len(waiting) >= 8 {
			break
		}
	}
	body, err := json.Marshal(map[string][]syncOperation{"operations": waiting})
	if err != nil {
		return err
	}
	response, err := accountCall(ctx, remote, http.MethodPost, "/api/sync", bytes.NewReader(body), "application/json")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return &deviceSyncHTTPError{response.StatusCode}
	}
	var result syncResult
	if err = json.NewDecoder(io.LimitReader(response.Body, 96*1024*1024)).Decode(&result); err != nil {
		return err
	}
	data.Snapshot = collectionState{Entries: result.Entries, Lists: result.Lists, Undo: result.Undo}
	data.Pending = slices.DeleteFunc(data.Pending, func(op syncOperation) bool { return slices.Contains(result.Accepted, op.ID) })
	data.Conflicts = append(data.Conflicts, result.Conflicts...)
	return writeJSON(path, data)
}

type deviceSyncHTTPError struct{ status int }

func (e *deviceSyncHTTPError) Error() string { return "Kontoserver hat den Abgleich abgelehnt." }

func (a *app) offlineAccount(w http.ResponseWriter, r *http.Request, remote desktopAccount) bool {
	path := r.URL.Path
	if path != "/api/collection" && path != "/api/entries" && path != "/api/settings" && path != "/api/goals" && path != "/api/device-sync" {
		return false
	}
	if (path == "/api/settings" || path == "/api/goals") && r.Method != http.MethodGet {
		return false
	}
	a.syncDeviceMu.Lock()
	defer a.syncDeviceMu.Unlock()
	data, file, err := a.readDeviceSync(remote)
	if err != nil {
		fail(w, 500, "Der lokale Abgleichsspeicher konnte nicht gelesen werden.")
		return true
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	if path == "/api/settings" || path == "/api/goals" {
		snapshot := &data.Preferences
		if path == "/api/goals" {
			snapshot = &data.Goals
		}
		response, callErr := accountCall(ctx, remote, http.MethodGet, path, nil, "")
		if callErr == nil {
			defer response.Body.Close()
			if response.StatusCode == 200 {
				raw, err := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024))
				if err == nil && json.Valid(raw) {
					*snapshot = raw
					_ = writeJSON(file, data)
				}
			}
		}
		if len(*snapshot) > 0 {
			reply(w, 200, *snapshot)
			return true
		}
		if callErr == nil {
			copyAccountResponse(w, response)
			return true
		}
		fail(w, 502, "Der Kontoserver ist nicht erreichbar.")
		return true
	}
	if path == "/api/device-sync" {
		if r.Method == http.MethodPost {
			var command struct {
				ID        string
				KeepLocal bool
			}
			if !decode(w, r, &command) {
				return true
			}
			if !data.resolve(command.ID, command.KeepLocal) {
				fail(w, 404, "Diese Änderung steht nicht mehr aus.")
				return true
			}
			if err = writeJSON(file, data); err != nil {
				fail(w, 500, "Der lokale Abgleich konnte nicht gespeichert werden.")
				return true
			}
		}
		reply(w, 200, map[string]any{"pending": len(data.Pending), "conflicts": data.Conflicts})
		return true
	}
	if r.Method == http.MethodGet {
		err = runDeviceSync(ctx, remote, &data, file)
		if data.Snapshot.Entries == nil && err != nil {
			var status *deviceSyncHTTPError
			if errors.As(err, &status) && status.status == 401 {
				fail(w, 401, "Melde dich erneut an.")
				return true
			}
			fail(w, 502, "Der Kontoserver ist nicht erreichbar.")
			return true
		}
		view := data.visible()
		if path == "/api/entries" {
			reply(w, 200, view.Entries)
		} else {
			reply(w, 200, view)
		}
		return true
	}
	if r.Method != http.MethodPut && r.Method != http.MethodDelete {
		return false
	}
	var item entry
	if !decodeLimited(w, r, &item, 512*1024) {
		return true
	}
	if data.Snapshot.Entries == nil {
		if err = runDeviceSync(ctx, remote, &data, file); err != nil {
			fail(w, 502, "Öffne die Sammlung zuerst einmal mit Internetverbindung.")
			return true
		}
	}
	if len(data.Pending) >= 4096 {
		fail(w, 409, "Zu viele offene Änderungen. Stelle die Verbindung zum Kontoserver wieder her.")
		return true
	}
	view := data.visible()
	base := findEntry(view.Entries, item.ID)
	id, err := randomToken()
	if err != nil {
		fail(w, 500, "Änderung konnte nicht vorgemerkt werden.")
		return true
	}
	operation := syncOperation{ID: id, ItemID: item.ID, Entry: &item, Base: base, Action: "add"}
	if r.Method == http.MethodDelete {
		if base == nil {
			reply(w, 200, true)
			return true
		}
		operation.Action = "delete"
		operation.Entry = nil
	} else if base != nil {
		if item.Revision != base.Revision {
			fail(w, 409, "Dieser Titel wurde auf einem anderen Gerät geändert. Öffne ihn erneut.")
			return true
		}
		operation.Action = "edit"
		operation.Fields = syncChangedFields(base, &item)
		if len(operation.Fields) == 0 {
			reply(w, 200, true)
			return true
		}
	} else if item.Revision != "" {
		fail(w, 409, "Dieser Titel wurde auf einem anderen Gerät geändert. Öffne ihn erneut.")
		return true
	}
	if operation.Action != "delete" && !valid(item) || !validSyncOperation(operation) {
		fail(w, 400, "Prüfe Titel, Status, Fortschritt und Bewertung.")
		return true
	}
	data.Pending = append(data.Pending, operation)
	if err = writeJSON(file, data); err != nil {
		fail(w, 500, "Der lokale Abgleich konnte nicht gespeichert werden.")
		return true
	}
	_ = runDeviceSync(ctx, remote, &data, file)
	reply(w, 200, true)
	return true
}

func (d *deviceSync) resolve(id string, keepLocal bool) bool {
	conflictIndex := slices.IndexFunc(d.Conflicts, func(c syncConflict) bool { return c.ID == id })
	pendingIndex := slices.IndexFunc(d.Pending, func(op syncOperation) bool { return op.ID == id })
	if conflictIndex < 0 || pendingIndex < 0 {
		return false
	}
	conflict := d.Conflicts[conflictIndex]
	itemID := d.Pending[pendingIndex].ItemID
	intended := findEntry(d.visible().Entries, itemID)
	var replacement *syncOperation
	if keepLocal {
		identifier, err := randomToken()
		if err != nil {
			return false
		}
		switch {
		case conflict.Current != nil && intended != nil:
			fields := syncChangedFields(conflict.Current, intended)
			if len(fields) > 0 {
				replacement = &syncOperation{ID: identifier, ItemID: itemID, Action: "edit", Base: conflict.Current, Entry: intended, Fields: fields}
			}
		case conflict.Current != nil:
			replacement = &syncOperation{ID: identifier, ItemID: itemID, Action: "delete", Base: conflict.Current}
		case intended != nil:
			intended.ID = "manual:" + identifier
			intended.Revision = ""
			replacement = &syncOperation{ID: identifier, ItemID: intended.ID, Action: "add", Entry: intended}
		}
	}
	removed := map[string]bool{}
	d.Pending = slices.DeleteFunc(d.Pending, func(op syncOperation) bool {
		if op.ItemID != itemID {
			return false
		}
		removed[op.ID] = true
		return true
	})
	d.Conflicts = slices.DeleteFunc(d.Conflicts, func(conflict syncConflict) bool { return removed[conflict.ID] })
	if replacement != nil {
		d.Pending = append(d.Pending, *replacement)
	}
	return true
}
