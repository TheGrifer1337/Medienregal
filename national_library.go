package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

var pageCount = regexp.MustCompile(`(?i)([0-9]+)\s*(?:S\.|Seiten|pages|Blatt|p\.)`)
var isbnPattern = regexp.MustCompile(`^(?:[0-9]{13}|[0-9]{9}[0-9X])$`)

type marcRecord struct {
	Number []struct {
		Tag   string `xml:"tag,attr"`
		Value string `xml:",chardata"`
	} `xml:"controlfield"`
	Fields []struct {
		Tag       string `xml:"tag,attr"`
		Subfields []struct {
			Code  string `xml:"code,attr"`
			Value string `xml:",chardata"`
		} `xml:"subfield"`
	} `xml:"datafield"`
}

func (record marcRecord) internal() (entry, string, string) {
	values := map[string]string{}
	genres := []string{}
	for _, field := range record.Fields {
		for _, sub := range field.Subfields {
			key := field.Tag + sub.Code
			value := strings.TrimSpace(sub.Value)
			if values[key] == "" {
				values[key] = value
			}
			if field.Tag == "655" || field.Tag == "650" {
				lower := strings.ToLower(value)
				for _, genre := range []string{"manga", "fantasy", "horror", "science fiction", "thriller", "krimi", "mystery", "romance", "abenteuer"} {
					if strings.Contains(lower, genre) {
						genres = append(genres, genre)
					}
				}
			}
		}
	}
	isbn := ""
	for _, field := range record.Fields {
		if field.Tag != "020" {
			continue
		}
		for _, sub := range field.Subfields {
			if sub.Code != "a" {
				continue
			}
			candidate := strings.ReplaceAll(strings.TrimSpace(sub.Value), "-", "")
			if isbnPattern.MatchString(candidate) && (isbn == "" || len(candidate) > len(isbn)) {
				isbn = candidate
			}
		}
	}
	id := ""
	for _, control := range record.Number {
		if control.Tag == "001" {
			id = control.Value
			break
		}
	}
	name := strings.Trim(values["245a"], " /:;")
	part := strings.Trim(values["245n"], " /:;")
	if part == "" {
		part = strings.Trim(values["490v"], " /:;")
	}
	if part != "" && !strings.Contains(strings.ToLower(name), strings.ToLower(part)) {
		name += " · " + part
	}
	if name == "" || id == "" || isbn == "" {
		return entry{}, "", ""
	}
	pages := 0
	if match := pageCount.FindStringSubmatch(values["300a"]); len(match) == 2 {
		pages, _ = strconv.Atoi(match[1])
	}
	author := values["100a"]
	if author == "" {
		author = values["245c"]
	}
	publisher := values["264b"]
	if publisher == "" {
		publisher = values["260b"]
	}
	subtitle := strings.Trim(strings.Join([]string{author, publisher, values["264c"]}, " · "), " ·")
	return entry{ID: "book:dnb:" + id, Kind: "book", Title: name, Subtitle: subtitle, Genres: prettyGenres(genres), Cover: "https://covers.openlibrary.org/b/isbn/" + isbn + "-M.jpg?default=false", Total: pages, Source: "Deutsche Nationalbibliothek"}, isbn, publisher
}

func cql(index, value string) string {
	clean := strings.NewReplacer(`"`, " ", `\`, " ", "*", " ", "?", " ").Replace(strings.TrimSpace(value))
	return index + ` all "` + clean + `"`
}

func searchDNB(ctx context.Context, query url.Values) ([]entry, error) {
	title := strings.TrimSpace(query.Get("q"))
	isbn := strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(title))
	terms := []string{}
	if isbnPattern.MatchString(isbn) {
		terms = append(terms, "isbn="+isbn)
	} else if title != "" {
		terms = append(terms, cql("tit", title))
	}
	for _, field := range []struct{ name, index string }{{"author", "per"}, {"publisher", "woe"}, {"volume", "tit"}} {
		if value := strings.TrimSpace(query.Get(field.name)); value != "" {
			terms = append(terms, cql(field.index, value))
		}
	}
	q := url.Values{"version": {"1.1"}, "operation": {"searchRetrieve"}, "query": {strings.Join(terms, " and ")}, "maximumRecords": {"20"}, "recordSchema": {"MARC21-xml"}}
	request, err := http.NewRequestWithContext(ctx, "GET", "https://services.dnb.de/sru/dnb?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "Medienregal/"+version+" (desktop application)")
	response, err := webClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DNB HTTP %d", response.StatusCode)
	}
	var payload struct {
		Records     []marcRecord `xml:"records>record>recordData>record"`
		Diagnostics []struct {
			Message string `xml:"message"`
		} `xml:"diagnostics>diagnostic"`
	}
	if err = xml.NewDecoder(io.LimitReader(response.Body, 8*1024*1024)).Decode(&payload); err != nil {
		return nil, err
	}
	if len(payload.Diagnostics) > 0 {
		return nil, fmt.Errorf("DNB: %s", payload.Diagnostics[0].Message)
	}
	items := []entry{}
	seen := map[string]bool{}
	for _, record := range payload.Records {
		item, isbn, publisher := record.internal()
		requiredPublisher := strings.TrimSpace(query.Get("publisher"))
		if requiredPublisher != "" && !strings.Contains(strings.ToLower(publisher), strings.ToLower(requiredPublisher)) {
			continue
		}
		if isbn != "" && !seen[isbn] {
			items = append(items, item)
			seen[isbn] = true
		}
	}
	if target := strings.TrimSpace(query.Get("volume")); target != "" {
		normalized := func(title string) string {
			return strings.ToLower(strings.Join(strings.Fields(strings.NewReplacer("·", " ", "-", " ", ".", " ", ":", " ").Replace(title)), " "))
		}
		target = normalized(target)
		sort.SliceStable(items, func(i, j int) bool {
			return normalized(items[i].Title) == target && normalized(items[j].Title) != target
		})
	}
	return items, nil
}

func combinedBooks(ctx context.Context, query url.Values) ([]entry, error) {
	var open, national []entry
	var openErr, nationalErr error
	var wait sync.WaitGroup
	wait.Add(2)
	go func() { defer wait.Done(); open, openErr = searchBooks(ctx, query) }()
	go func() { defer wait.Done(); national, nationalErr = searchDNB(ctx, query) }()
	wait.Wait()
	if openErr != nil && nationalErr != nil {
		return nil, openErr
	}
	result := append(national, open...)
	if result == nil {
		result = []entry{}
	}
	return result, nil
}
