package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (a *app) desktopStatus() accountStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	result := accountStatus{Platform: "desktop", Server: a.remote.Server, Name: a.remote.Name, LocalCount: len(a.data.Entries)}
	if a.host != nil {
		result.Invite = a.host.invite
		result.HostActive = true
		if result.Server == "" {
			result.Server = "http://" + accountPort
		}
	}
	return result
}

func (a *app) desktopAccount(w http.ResponseWriter, r *http.Request) {
	if a.hosted {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodGet {
		reply(w, 200, a.desktopStatus())
		return
	}
	var request accountRequest
	if !decodeLimited(w, r, &request, 8192) {
		return
	}
	a.accountActions.Lock()
	defer a.accountActions.Unlock()
	folder := filepath.Dir(a.path)
	switch request.Action {
	case "host":
		if a.host == nil {
			host, server, err := listenAccountHost(filepath.Join(folder, "Kontoserver"))
			if err != nil {
				fail(w, 409, "Der Kontoserver konnte nicht gestartet werden. Prüfe Port 8765.")
				return
			}
			a.mu.Lock()
			a.host, a.hostHTTP = host, server
			a.mu.Unlock()
			_ = os.WriteFile(filepath.Join(folder, "account-host.enabled"), []byte("enabled\n"), 0600)
		}
	case "logout":
		a.mu.Lock()
		remote := a.remote
		a.mu.Unlock()
		if err := os.Remove(filepath.Join(folder, "device-account.json")); err != nil && !os.IsNotExist(err) {
			fail(w, 500, "Abmeldung fehlgeschlagen.")
			return
		}
		a.mu.Lock()
		a.remote = desktopAccount{}
		a.mu.Unlock()
		if remote.Token != "" {
			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()
			if response, err := accountCall(ctx, remote, http.MethodPost, "/api/account/logout", nil, ""); err == nil {
				_ = response.Body.Close()
			}
		}
	case "login", "register":
		server := validAccountServer(request.Server)
		if server == "" {
			fail(w, 400, "Gib die HTTPS-Adresse deines Kontoservers ein.")
			return
		}
		remote := desktopAccount{Server: server}
		request.Native = true
		transfer := request.Import
		request.Server = ""
		request.Import = false
		raw, err := json.Marshal(request)
		if err != nil {
			fail(w, 400, "Die Anmeldung konnte nicht gelesen werden.")
			return
		}
		response, err := accountCall(r.Context(), remote, http.MethodPost, "/api/account/session", bytes.NewReader(raw), "application/json")
		if err != nil {
			fail(w, 502, "Der Kontoserver ist nicht erreichbar.")
			return
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			copyAccountResponse(w, response)
			return
		}
		if json.NewDecoder(io.LimitReader(response.Body, 8192)).Decode(&remote) != nil || !opaqueToken.MatchString(remote.Token) || !accountName.MatchString(remote.Name) {
			fail(w, 502, "Der Server hat keine gültige Anmeldung zurückgegeben.")
			return
		}
		remote.Server = server
		if transfer {
			a.mu.Lock()
			data := library{Entries: a.data.Entries, Lists: a.data.Lists, Goals: a.data.Goals}
			payload, err := json.Marshal(data)
			a.mu.Unlock()
			if err != nil || len(payload) > 32*1024*1024 {
				fail(w, 400, "Die lokale Sammlung ist zu groß.")
				return
			}
			result, err := accountCall(r.Context(), remote, http.MethodPut, "/api/account/import", bytes.NewReader(payload), "application/json")
			if err != nil {
				fail(w, 502, "Der Kontoserver ist nicht erreichbar.")
				return
			}
			defer result.Body.Close()
			if result.StatusCode != 200 {
				copyAccountResponse(w, result)
				return
			}
		}
		if err = writeJSON(filepath.Join(folder, "device-account.json"), remote); err != nil {
			fail(w, 500, "Die Anmeldung konnte nicht gespeichert werden.")
			return
		}
		a.mu.Lock()
		a.remote = remote
		a.mu.Unlock()
	default:
		fail(w, 400, "Unbekannte Kontoaktion.")
		return
	}
	reply(w, 200, a.desktopStatus())
}

func copyAccountResponse(w http.ResponseWriter, response *http.Response) {
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		fail(w, 502, "Der Kontoserver hat eine unerwartete Weiterleitung gesendet.")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
}

func (a *app) withAccountProxy(local http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if !strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/api/account") || desktopOnly(path) {
			local.ServeHTTP(w, r)
			return
		}
		a.mu.Lock()
		remote := a.remote
		a.mu.Unlock()
		if remote.Token == "" {
			local.ServeHTTP(w, r)
			return
		}
		response, err := accountCall(r.Context(), remote, r.Method, r.URL.RequestURI(), http.MaxBytesReader(w, r.Body, 8*1024*1024), r.Header.Get("Content-Type"))
		if err != nil {
			fail(w, 502, "Der Kontoserver ist nicht erreichbar.")
			return
		}
		defer response.Body.Close()
		copyAccountResponse(w, response)
	})
}
