package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

var releaseTag = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.([0-9])$`)
var sha256Digest = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
var repoName = regexp.MustCompile(`^[a-zA-Z0-9-]+/[a-zA-Z0-9_.-]+$`)

type releaseAsset struct {
	Name, Digest, State string
	URL                 string `json:"browser_download_url"`
	Size                int64
}

type release struct {
	Tag               string `json:"tag_name"`
	Draft, Prerelease bool
	Assets            []releaseAsset
}

func versionNumber(tag string) int {
	if !releaseTag.MatchString(tag) {
		return -1
	}
	value, err := strconv.Atoi(strings.ReplaceAll(tag[1:], ".", ""))
	if err != nil {
		return -1
	}
	return value
}

func bestUpdate(releases []release, current int, best int, repo string) (int, releaseAsset) {
	var selected releaseAsset
	for _, rel := range releases {
		next := versionNumber(rel.Tag)
		if rel.Draft || rel.Prerelease || next <= current || next <= best {
			continue
		}
		name := "Medienregal-Setup-" + rel.Tag + ".exe"
		for _, asset := range rel.Assets {
			expected := "https://github.com/" + repo + "/releases/download/" + rel.Tag + "/" + name
			if asset.Name == name && asset.URL == expected && asset.State == "uploaded" && sha256Digest.MatchString(asset.Digest) && asset.Size > 0 && asset.Size <= 100*1024*1024 {
				best, selected = next, asset
				break
			}
		}
	}
	return best, selected
}

type updateStatus struct {
	State   string `json:"state"`
	Version string `json:"version,omitempty"`
	Message string `json:"message,omitempty"`
}

type updateCheck struct {
	mu        sync.Mutex
	status    updateStatus
	installer string
	asset     releaseAsset
}

func (u *updateCheck) snapshot() updateStatus {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.status
}

func (u *updateCheck) start(ctx context.Context) updateStatus {
	u.mu.Lock()
	defer u.mu.Unlock()
	switch u.status.State {
	case "checking", "downloading", "ready":
		return u.status
	}
	u.status = updateStatus{State: "checking"}
	go u.run(ctx)
	return u.status
}

func (u *updateCheck) run(ctx context.Context) {
	status, path, asset := stageUpdate(ctx, func(next string) {
		u.mu.Lock()
		u.status = updateStatus{State: "downloading", Version: next}
		u.mu.Unlock()
	})
	u.mu.Lock()
	defer u.mu.Unlock()
	u.status, u.installer, u.asset = status, path, asset
}

func (u *updateCheck) apply() error {
	u.mu.Lock()
	path, asset := u.installer, u.asset
	u.mu.Unlock()
	if path == "" {
		return nil
	}
	if !verifyInstaller(path, asset) {
		return errors.New("Die heruntergeladene Update-Datei ist beschädigt. Suche erneut nach Updates.")
	}
	return applyUpdate(path)
}

func stageUpdate(ctx context.Context, downloading func(string)) (updateStatus, string, releaseAsset) {
	unable := updateStatus{State: "unavailable", Message: "Automatische Updates sind für die installierte Windows-Version verfügbar."}
	if runtime.GOOS != "windows" || !repoName.MatchString(releaseRepo) {
		return unable, "", releaseAsset{}
	}
	exe, err := os.Executable()
	if err != nil {
		return unable, "", releaseAsset{}
	}
	if _, err = os.Stat(filepath.Join(filepath.Dir(exe), "Medienregal-Deinstallieren.exe")); err != nil {
		return unable, "", releaseAsset{}
	}
	current := versionNumber("v" + version)
	best := current
	var selected releaseAsset
	for page := 1; ; page++ {
		var releases []release
		address := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=100&page=%d", releaseRepo, page)
		if err := getJSON(ctx, address, "", &releases); err != nil {
			return updateStatus{State: "error", Message: "Die Updatesuche konnte GitHub nicht erreichen. Versuche es erneut."}, "", releaseAsset{}
		}
		next, asset := bestUpdate(releases, current, best, releaseRepo)
		if next > best {
			best, selected = next, asset
		}
		if len(releases) < 100 {
			break
		}
	}
	if selected.URL == "" {
		return updateStatus{State: "current", Version: version}, "", releaseAsset{}
	}
	nextVersion := fmt.Sprintf("%d.%d", best/10, best%10)
	downloadError := updateStatus{State: "error", Version: nextVersion, Message: "Das Update konnte nicht geladen werden. Versuche es erneut."}
	base, err := os.UserCacheDir()
	if err != nil {
		return downloadError, "", releaseAsset{}
	}
	dir := filepath.Join(base, product, "Updates")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return downloadError, "", releaseAsset{}
	}
	path := filepath.Join(dir, selected.Name)
	if !verifyInstaller(path, selected) {
		downloading(nextVersion)
		if err = downloadInstaller(ctx, path, selected); err != nil {
			return downloadError, "", releaseAsset{}
		}
	}
	return updateStatus{State: "ready", Version: nextVersion}, path, selected
}

func verifyInstaller(path string, asset releaseAsset) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	sum := sha256.New()
	size, err := io.Copy(sum, io.LimitReader(file, asset.Size+1))
	return err == nil && size == asset.Size && "sha256:"+hex.EncodeToString(sum.Sum(nil)) == asset.Digest
}

func downloadInstaller(ctx context.Context, path string, asset releaseAsset) error {
	client := &http.Client{Timeout: 3 * time.Minute, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		h := r.URL.Hostname()
		if r.URL.Scheme != "https" || len(via) > 5 || (h != "github.com" && h != "release-assets.githubusercontent.com" && h != "objects.githubusercontent.com") {
			return errors.New("unzulässige Weiterleitung")
		}
		return nil
	}}
	address, err := url.Parse(asset.URL)
	if err != nil || address.Host != "github.com" || address.Scheme != "https" {
		return errors.New("ungültige Download-Adresse")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", asset.URL, nil)
	if err != nil {
		return err
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	file, err := os.CreateTemp(filepath.Dir(path), "update-*.part")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	size, err := io.Copy(file, io.LimitReader(res.Body, asset.Size+1))
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if size != asset.Size || !verifyInstaller(file.Name(), asset) {
		return errors.New("ungültiger Download")
	}
	return os.Rename(file.Name(), path)
}
