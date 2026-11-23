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

func stageUpdate(ctx context.Context) string {
	if runtime.GOOS != "windows" || !repoName.MatchString(releaseRepo) {
		return ""
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if _, err = os.Stat(filepath.Join(filepath.Dir(exe), "Medienregal-Deinstallieren.exe")); err != nil {
		return ""
	}
	current := versionNumber("v" + version)
	best := current
	var selected releaseAsset
	for page := 1; ; page++ {
		var releases []release
		address := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=100&page=%d", releaseRepo, page)
		if err := getJSON(ctx, address, "", &releases); err != nil {
			return ""
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
		return ""
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(base, product, "Updates")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return ""
	}
	path := filepath.Join(dir, selected.Name)
	if verifyInstaller(path, selected) {
		return path
	}
	if err = downloadInstaller(ctx, path, selected); err != nil {
		return ""
	}
	return path
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
