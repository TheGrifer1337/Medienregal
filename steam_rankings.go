package main

import (
	"context"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var steamChartTags = map[string]string{
	"Abenteuer": "21", "Action": "19", "Horror": "1667", "Jump ’n’ Run": "1625",
	"Rennspiel": "699", "Rollenspiel": "122", "Simulation": "599", "Strategie": "9",
}

var steamSearchRows = regexp.MustCompile(`(?s)<a\s[^>]*data-ds-appid="([0-9]+)"[^>]*>(.*?)</a>`)
var steamSearchName = regexp.MustCompile(`(?s)<span class="title">([^<]+)</span>`)
var steamSearchDate = regexp.MustCompile(`(?s)<div class="search_released[^"]*">\s*([^<]+)</div>`)

func steamChart(ctx context.Context, year int, genre string) (annualChart, error) {
	chart := annualChart{Year: year, Source: "Steam · neueste Veröffentlichungen", Items: []entry{}}
	params := url.Values{"query": {""}, "start": {"0"}, "count": {"100"}, "sort_by": {"Released_DESC"}, "category1": {"998"}, "ndl": {"1"}, "infinite": {"1"}, "l": {"english"}}
	if tag := steamChartTags[genre]; tag != "" {
		params.Set("tags", tag)
	}
	var payload struct {
		Results string `json:"results_html"`
	}
	err := getJSON(ctx, "https://store.steampowered.com/search/results/?"+params.Encode(), "", &payload)
	if err != nil {
		return chart, err
	}
	seen := map[string]bool{}
	for _, row := range steamSearchRows.FindAllStringSubmatch(payload.Results, -1) {
		name, date := steamSearchName.FindStringSubmatch(row[2]), steamSearchDate.FindStringSubmatch(row[2])
		if len(name) == 0 || len(date) == 0 || seen[row[1]] {
			continue
		}
		if !strings.HasSuffix(strings.TrimSpace(date[1]), strconv.Itoa(year)) {
			continue
		}
		released, err := time.Parse("2 Jan, 2006", strings.TrimSpace(date[1]))
		if err != nil {
			released, err = time.Parse("Jan 2, 2006", strings.TrimSpace(date[1]))
		}
		if err != nil || released.After(time.Now()) {
			continue
		}
		seen[row[1]] = true
		var genres []string
		if genre != "" {
			genres = []string{genre}
		}
		chart.Items = append(chart.Items, entry{ID: "game:steam:" + row[1], Kind: "game", Title: html.UnescapeString(name[1]),
			Subtitle: released.Format("02.01.2006"), Cover: "https://cdn.cloudflare.steamstatic.com/steam/apps/" + row[1] + "/header.jpg", Genres: genres, Source: "Steam"})
		if len(chart.Items) == 50 {
			break
		}
	}
	return chart, nil
}
