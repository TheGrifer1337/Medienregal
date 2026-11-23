package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
)

type coverCandidate struct {
	URL    string `json:"url"`
	Source string `json:"source"`
	Page   string `json:"page,omitempty"`
}

type coverQuery struct {
	Kind, Title, Subtitle, ID, ISBN string
	Aliases                         []string
}

var coverYear = regexp.MustCompile(`^\s*((?:18|19|20)[0-9]{2})(?:$|[^0-9])`)

func (a *app) lookupCover(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	query := coverQuery{
		Kind: params.Get("kind"), Title: strings.TrimSpace(params.Get("title")), Subtitle: params.Get("subtitle"),
		ID: params.Get("id"), ISBN: params.Get("isbn"), Aliases: params["alias"],
	}
	if _, ok := statuses[query.Kind]; !ok || query.Title == "" || len(query.Title) > 500 || len(r.URL.RawQuery) > 4000 {
		fail(w, 400, "Für die Coversuche wird ein Titel benötigt.")
		return
	}
	if !isbnPattern.MatchString(query.ISBN) {
		query.ISBN = ""
	}
	ctx, cancel := context.WithTimeout(r.Context(), 7*time.Second)
	defer cancel()
	reply(w, http.StatusOK, query.search(ctx, a.getSettings()))
}

func (q coverQuery) search(ctx context.Context, prefs settings) []coverCandidate {
	type lookup func() coverCandidate
	var sources []lookup
	switch q.Kind {
	case "book":
		sources = append(sources, func() coverCandidate { return q.googleBooks(ctx) }, func() coverCandidate { return q.openLibrary(ctx) })
	case "movie", "series":
		if q.Kind == "series" {
			sources = append(sources, func() coverCandidate { return q.tvmaze(ctx) })
		}
		if prefs.TMDB != "" {
			sources = append(sources, func() coverCandidate {
				items, _ := searchTMDB(ctx, q.Kind, q.Title, prefs.TMDB, prefs.tmdbLocale())
				return q.catalogCover(items)
			})
		}
		sources = append(sources, func() coverCandidate {
			if id, ok := q.catalogKey("imdb"); ok && imdbID.MatchString(id) {
				return coverCandidate{URL: "https://images.metahub.space/poster/medium/" + id + "/img", Source: "MetaHub"}
			}
			items, _ := searchCinemeta(ctx, q.Kind, q.Title)
			return q.catalogCover(items)
		}, func() coverCandidate { return q.wikimedia(ctx) })
	case "game":
		sources = append(sources, func() coverCandidate {
			if id, ok := q.catalogKey("steam"); ok && numericID.MatchString(id) {
				return coverCandidate{URL: "https://shared.fastly.steamstatic.com/store_item_assets/steam/apps/" + id + "/library_600x900.jpg", Source: "Steam"}
			}
			items, _ := searchGames(ctx, q.Title, prefs.Region, prefs.steamLocale())
			return q.catalogCover(items)
		}, func() coverCandidate { return q.wikimedia(ctx) })
	}
	results := make([]chan coverCandidate, len(sources))
	for i, source := range sources {
		results[i] = make(chan coverCandidate, 1)
		go func() { results[i] <- source() }()
	}
	found := []coverCandidate{}
	seen := map[string]bool{}
	for _, result := range results {
		var candidate coverCandidate
		select {
		case candidate = <-result:
		case <-ctx.Done():
			// Completed sources can still return results once the deadline expires.
			select {
			case candidate = <-result:
			default:
			}
		}
		if image := secureCoverURL(candidate.URL); image != "" && !seen[image] {
			candidate.URL, candidate.Page = image, secureCoverURL(candidate.Page)
			found = append(found, candidate)
			seen[image] = true
		}
	}
	return found
}

func secureCoverURL(value string) string {
	address, err := url.Parse(value)
	if err != nil || address.Scheme != "https" || address.Hostname() == "" || address.User != nil {
		return ""
	}
	return address.String()
}

func (q coverQuery) catalogKey(source string) (string, bool) {
	return strings.CutPrefix(q.ID, q.Kind+":"+source+":")
}

func coverWords(value string) []string {
	return strings.FieldsFunc(strings.ToLower(value), func(char rune) bool {
		return !unicode.IsLetter(char) && !unicode.IsNumber(char)
	})
}

func coverName(value string) string {
	return strings.Join(coverWords(value), " ")
}

func (q coverQuery) matchesTitle(title string, aliases ...string) bool {
	for _, candidate := range append([]string{title}, aliases...) {
		name := coverName(candidate)
		if name == "" {
			continue
		}
		if coverName(q.Title) == name || slices.ContainsFunc(q.Aliases, func(alias string) bool { return coverName(alias) == name }) {
			return true
		}
	}
	return false
}

func (q coverQuery) matchesYear(date string) bool {
	want, got := coverYear.FindStringSubmatch(q.Subtitle), coverYear.FindStringSubmatch(date)
	return len(want) == 0 || len(got) > 0 && want[1] == got[1]
}

func (q coverQuery) catalogCover(items []entry) coverCandidate {
	for _, item := range items {
		if item.ID == q.ID {
			return coverCandidate{URL: item.Cover, Source: item.Source}
		}
	}
	var match *entry
	for _, item := range items {
		if !q.matchesTitle(item.Title, item.Aliases...) || q.Kind != "game" && !q.matchesYear(item.Subtitle) {
			continue
		}
		if match != nil && match.ID != item.ID {
			return coverCandidate{}
		}
		match = &item
	}
	if match != nil {
		return coverCandidate{URL: match.Cover, Source: match.Source}
	}
	return coverCandidate{}
}

func bookAuthorKey(author string) string {
	words := coverWords(author)
	slices.Sort(words)
	return strings.Join(words, " ")
}

type bookArtwork struct {
	title, image, page string
	authors, isbns     []string
}

func (q coverQuery) bookCover(items []bookArtwork, source string) coverCandidate {
	author, _, _ := strings.Cut(q.Subtitle, " · ")
	wantAuthor := bookAuthorKey(author)
	var match coverCandidate
	var matchedAuthor string
	var matchedBook bool
	for _, item := range items {
		if q.ISBN != "" {
			if !slices.Contains(item.isbns, q.ISBN) {
				continue
			}
		} else {
			if !q.matchesTitle(item.title) {
				continue
			}
			if wantAuthor != "" && !slices.ContainsFunc(item.authors, func(author string) bool { return bookAuthorKey(author) == wantAuthor }) &&
				bookAuthorKey(strings.Join(item.authors, " ")) != wantAuthor {
				continue
			}
			if wantAuthor == "" {
				key := bookAuthorKey(strings.Join(item.authors, " "))
				if matchedBook && matchedAuthor != key {
					return coverCandidate{}
				}
				matchedAuthor = key
				matchedBook = true
			}
		}
		if match.URL == "" && secureCoverURL(item.image) != "" {
			match = coverCandidate{URL: item.image, Source: source, Page: item.page}
		}
	}
	return match
}

func (q coverQuery) googleBooks(ctx context.Context) coverCandidate {
	term := "intitle:" + q.Title
	if q.ISBN != "" {
		term = "isbn:" + q.ISBN
	}
	params := url.Values{"q": {term}, "maxResults": {"15"}, "fields": {"items(volumeInfo(title,subtitle,authors,imageLinks,industryIdentifiers,infoLink))"}}
	var response struct {
		Items []struct {
			VolumeInfo struct {
				Title, Subtitle, InfoLink string
				Authors                   []string
				ImageLinks                struct{ Thumbnail string }
				IndustryIdentifiers       []struct{ Identifier string }
			}
		}
	}
	_ = getJSON(ctx, "https://www.googleapis.com/books/v1/volumes?"+params.Encode(), "", &response)
	items := []bookArtwork{}
	for _, item := range response.Items {
		data := item.VolumeInfo
		title := data.Title
		if data.Subtitle != "" && q.matchesTitle(title+" "+data.Subtitle) {
			title += " " + data.Subtitle
		}
		result := bookArtwork{title: title, image: googleBookHTTPS(data.ImageLinks.Thumbnail), page: googleBookHTTPS(data.InfoLink), authors: data.Authors}
		for _, isbn := range data.IndustryIdentifiers {
			result.isbns = append(result.isbns, isbn.Identifier)
		}
		items = append(items, result)
	}
	return q.bookCover(items, "Google Books")
}

func googleBookHTTPS(value string) string {
	if address, err := url.Parse(value); err == nil && address.Scheme == "http" && address.Hostname() == "books.google.com" {
		address.Scheme = "https"
		return address.String()
	}
	return value
}

func (q coverQuery) openLibrary(ctx context.Context) coverCandidate {
	if q.ISBN != "" {
		return coverCandidate{URL: "https://covers.openlibrary.org/b/isbn/" + q.ISBN + "-L.jpg?default=false", Source: "Open Library",
			Page: "https://openlibrary.org/isbn/" + q.ISBN}
	}
	params := url.Values{"title": {q.Title}, "fields": {"key,title,author_name,cover_i"}, "limit": {"15"}}
	var response struct {
		Docs []struct {
			Key, Title string
			Authors    []string `json:"author_name"`
			Cover      int      `json:"cover_i"`
		}
	}
	_ = getJSON(ctx, "https://openlibrary.org/search.json?"+params.Encode(), "", &response)
	items := []bookArtwork{}
	for _, item := range response.Docs {
		items = append(items, bookArtwork{title: item.Title, image: bookCover(item.Cover), page: "https://openlibrary.org/" + strings.TrimPrefix(item.Key, "/"), authors: item.Authors})
	}
	return q.bookCover(items, "Open Library")
}

type tvmazeArtwork struct {
	ID                   int
	Name, Premiered, URL string
	Externals            struct{ IMDb string }
	Image                struct{ Medium, Original string }
}

func (q coverQuery) tvmaze(ctx context.Context) coverCandidate {
	makeCover := func(show tvmazeArtwork) coverCandidate {
		image := show.Image.Medium
		if image == "" {
			image = show.Image.Original
		}
		return coverCandidate{URL: image, Source: "TVmaze", Page: show.URL}
	}
	if id, ok := q.catalogKey("imdb"); ok && imdbID.MatchString(id) {
		var show tvmazeArtwork
		_ = getJSON(ctx, "https://api.tvmaze.com/lookup/shows?imdb="+id, "", &show)
		if show.Externals.IMDb == id {
			return makeCover(show)
		}
		return coverCandidate{}
	}
	var shows []struct{ Show tvmazeArtwork }
	_ = getJSON(ctx, "https://api.tvmaze.com/search/shows?"+url.Values{"q": {q.Title}}.Encode(), "", &shows)
	var match tvmazeArtwork
	for _, result := range shows {
		show := result.Show
		if q.matchesTitle(show.Name) && q.matchesYear(show.Premiered) {
			if match.ID != 0 && match.ID != show.ID {
				return coverCandidate{}
			}
			match = show
		}
	}
	return makeCover(match)
}

type wikimediaEntity struct {
	Claims map[string][]struct {
		Rank     string
		MainSnak struct {
			DataValue struct {
				Value json.RawMessage
			}
		}
	}
}

func (entity wikimediaEntity) strings(property string) []string {
	values := []string{}
	for _, claim := range entity.Claims[property] {
		if claim.Rank == "deprecated" {
			continue
		}
		var store struct{ ID, Time string }
		var value string
		if json.Unmarshal(claim.MainSnak.DataValue.Value, &value) == nil {
			values = append(values, value)
		} else if json.Unmarshal(claim.MainSnak.DataValue.Value, &store) == nil {
			values = append(values, store.ID, strings.TrimPrefix(store.Time, "+"))
		}
	}
	return values
}

func (q coverQuery) wikimedia(ctx context.Context) coverCandidate {
	const endpoint = "https://www.wikidata.org/w/api.php?"
	var ids []string
	qid, exact := q.catalogKey("wikidata")
	if exact && wikidataID.MatchString(qid) {
		ids = []string{qid}
	} else {
		exact = false
		for _, lang := range []string{"de", "en"} {
			query := url.Values{"action": {"wbsearchentities"}, "search": {q.Title}, "language": {lang}, "format": {"json"}, "limit": {"10"}}
			var response struct {
				Search []struct {
					ID, Label string
					Match     struct{ Text string }
				}
			}
			_ = getJSON(ctx, endpoint+query.Encode(), "", &response)
			for _, item := range response.Search {
				if wikidataID.MatchString(item.ID) && q.matchesTitle(item.Label, item.Match.Text) && !slices.Contains(ids, item.ID) {
					ids = append(ids, item.ID)
				}
			}
		}
	}
	if len(ids) == 0 {
		return coverCandidate{}
	}
	params := url.Values{"action": {"wbgetentities"}, "format": {"json"}, "ids": {strings.Join(ids, "|")}, "props": {"claims"}}
	var response struct{ Entities map[string]wikimediaEntity }
	_ = getJSON(ctx, endpoint+params.Encode(), "", &response)
	var matches []wikimediaEntity
	for _, id := range ids {
		entity, ok := response.Entities[id]
		if !ok {
			continue
		}
		if exact || q.matchesEntity(entity) {
			matches = append(matches, entity)
		}
	}
	if len(matches) != 1 {
		return coverCandidate{}
	}
	properties := []string{"P3383"}
	if q.Kind == "game" {
		properties = []string{"P18"}
	}
	for _, property := range properties {
		for _, filename := range matches[0].strings(property) {
			if slices.Contains([]string{".jpg", ".jpeg", ".png", ".webp", ".svg"}, strings.ToLower(path.Ext(filename))) {
				file := url.PathEscape(strings.ReplaceAll(filename, " ", "_"))
				return coverCandidate{URL: "https://commons.wikimedia.org/wiki/Special:FilePath/" + file + "?width=400",
					Source: "Wikimedia Commons", Page: "https://commons.wikimedia.org/wiki/File:" + file}
			}
		}
	}
	return coverCandidate{}
}

func (q coverQuery) matchesEntity(entity wikimediaEntity) bool {
	for source, property := range map[string]string{"imdb": "P345", "steam": "P1733", "tmdb": map[string]string{"movie": "P4947", "series": "P4983"}[q.Kind]} {
		if id, ok := q.catalogKey(source); ok && property != "" {
			ids := entity.strings(property)
			if len(ids) > 0 {
				return slices.Contains(ids, id)
			}
		}
	}
	classes := map[string][]string{"movie": {"Q11424"}, "series": {"Q5398426", "Q1259759"}, "game": {"Q7889"}}
	if !slices.ContainsFunc(entity.strings("P31"), func(id string) bool { return slices.Contains(classes[q.Kind], id) }) {
		return false
	}
	return coverYear.FindString(q.Subtitle) == "" || slices.ContainsFunc(append(entity.strings("P577"), entity.strings("P580")...), func(year string) bool {
		return q.matchesYear(year)
	})
}
