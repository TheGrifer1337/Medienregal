package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type coverTestTransport func(*http.Request) (*http.Response, error)

func (transport coverTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return transport(r)
}

func TestCoverLookupSources(t *testing.T) {
	original := webClient
	t.Cleanup(func() { webClient = original })
	webClient = &http.Client{Transport: coverTestTransport(func(r *http.Request) (*http.Response, error) {
		var body string
		switch r.URL.Hostname() {
		case "www.googleapis.com":
			if r.URL.Query().Get("q") != "isbn:9780140328721" {
				return nil, fmt.Errorf("missing ISBN: %s", r.URL)
			}
			body = `{"items":[{"volumeInfo":{"title":"Other book","industryIdentifiers":[{"identifier":"9780140328722"}],"imageLinks":{"thumbnail":"https://books.google.com/incorrect"}}},{"volumeInfo":{"title":"Matilda","industryIdentifiers":[{"identifier":"9780140328721"}],"imageLinks":{"thumbnail":"http://books.google.com/correct"},"infoLink":"http://books.google.com/actual"}}]}`
		case "api.tvmaze.com":
			body = `{"externals":{"imdb":"tt1234567"},"image":{"medium":"https://static.tvmaze.com/show.jpg"},"url":"https://www.tvmaze.com/shows/1/show"}`
		case "www.wikidata.org":
			if r.URL.Query().Get("action") == "wbsearchentities" {
				body = `{"search":[{"id":"Q100","label":"The Film"}]}`
				break
			}
			if r.URL.Query().Get("ids") == "Q300" {
				body = `{"entities":{"Q300":{"claims":{"P18":[{"mainsnak":{"datavalue":{"value":"Game image.png"}}}]}}}}`
				break
			}
			body = `{"entities":{"Q100":{"claims":{"P31":[{"mainsnak":{"datavalue":{"value":{"id":"Q11424"}}}}],"P577":[{"mainsnak":{"datavalue":{"value":{"time":"+1930-01-01T00:00:00Z"}}}}],"P3383":[{"mainsnak":{"datavalue":{"value":"Film poster.jpg"}}}]}}}}`
		case "v3-cinemeta.strem.io":
			body = `{"metas":[{"imdb_id":"tt1234567","name":"The Film","releaseInfo":"2001","poster":"https://example.org/wrong-year.jpg"}]}`
		case "store.steampowered.com":
			body = `{"items":[{"id":123,"type":"app","name":"Similar Game (Demo)","tiny_image":"https://example.org/wrong-game.jpg"}]}`
		default:
			return nil, fmt.Errorf("unexpected request: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}

	handler := (&app{}).routes()
	for _, test := range []struct {
		kind, title, subtitle, id, isbn, source, image string
	}{
		{"book", "Matilda", "Roald Dahl", "", "9780140328721", "Google Books", "https://books.google.com/correct"},
		{"series", "One Show", "2019", "series:imdb:tt1234567", "", "TVmaze", "https://static.tvmaze.com/show.jpg"},
		{"movie", "The Film", "1930", "", "", "Wikimedia Commons", "https://commons.wikimedia.org/wiki/Special:FilePath/Film_poster.jpg?width=400"},
		{"game", "Similar Game", "", "game:wikidata:Q300", "", "Wikimedia Commons", "https://commons.wikimedia.org/wiki/Special:FilePath/Game_image.png?width=400"},
	} {
		t.Run(test.kind, func(t *testing.T) {
			params := url.Values{"kind": {test.kind}, "title": {test.title}, "subtitle": {test.subtitle}, "id": {test.id}, "isbn": {test.isbn}}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/cover?"+params.Encode(), nil))
			var candidates []coverCandidate
			if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &candidates) != nil || len(candidates) == 0 {
				t.Fatalf("cover lookup: %d %s", response.Code, response.Body.String())
			}
			if got := candidates[0]; got.Source != test.source || got.URL != test.image {
				t.Errorf("first alternative is %v, expected %s from %s", got, test.image, test.source)
			}
		})
	}
}

func TestCoverLookupTitleAmbiguity(t *testing.T) {
	a := entry{ID: "movie:imdb:tt100", Title: "King Kong", Subtitle: "1933", Cover: "https://example.org/1933.jpg"}
	b := entry{ID: "movie:imdb:tt101", Title: "King Kong", Subtitle: "2005", Cover: "https://example.org/2005.jpg"}
	query := coverQuery{Kind: "movie", Title: "King Kong"}
	if candidate := query.catalogCover([]entry{a, b}); candidate.URL != "" {
		t.Errorf("ambiguous remake returned %s", candidate.URL)
	}
	query.Subtitle = "2005"
	if candidate := query.catalogCover([]entry{a, b}); candidate.URL != b.Cover {
		t.Errorf("year matched %s, want %s", candidate.URL, b.Cover)
	}
	query.ID = a.ID
	if candidate := query.catalogCover([]entry{a, b}); candidate.URL != a.Cover {
		t.Errorf("identifier matched %s, want %s", candidate.URL, a.Cover)
	}
}
