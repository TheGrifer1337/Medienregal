package main

import (
	"strings"
	"testing"
)

func TestHighestStableUpdate(t *testing.T) {
	const repo = "example/Medienregal"
	withInstaller := func(tag string) release {
		name := "Medienregal-Setup-" + tag + ".exe"
		return release{Tag: tag, Assets: []releaseAsset{{
			Name: name, State: "uploaded", Size: 32,
			URL:    "https://github.com/" + repo + "/releases/download/" + tag + "/" + name,
			Digest: "sha256:" + strings.Repeat("a", 64),
		}}}
	}
	wrongHost := withInstaller("v2.4")
	wrongHost.Assets[0].URL = strings.Replace(wrongHost.Assets[0].URL, "github.com", "example.org", 1)
	noDigest := withInstaller("v2.3")
	noDigest.Assets[0].Digest = ""
	prerelease := withInstaller("v2.2")
	prerelease.Prerelease = true
	draft := withInstaller("v2.1")
	draft.Draft = true
	releases := []release{withInstaller("v1.7"), wrongHost, withInstaller("v2.0"), noDigest, draft, prerelease, withInstaller("v1.9")}
	best, asset := bestUpdate(releases, versionNumber("v1.9"), versionNumber("v1.9"), repo)
	if best != versionNumber("v2.0") || asset.Name != "Medienregal-Setup-v2.0.exe" {
		t.Fatalf("unexpected release: %d %v", best, asset)
	}
}

func TestOfferAggregationKeepsLowestReportedPrice(t *testing.T) {
	expensive, cheap := 14.99, 9.99
	items := mergedOffers([]offer{{Name: "Prime Video", Type: "buy", Price: &expensive},
		{Name: "Prime Video", Type: "buy", Price: &cheap}}, []offer{
		{Name: "Prime Video", Type: "buy"}, {Name: "Prime Video", Type: "sub"},
	})
	if len(items) != 2 || items[0].Price == nil || *items[0].Price != cheap || items[1].Type != "sub" {
		t.Fatalf("unexpected offers: %+v", items)
	}
}
