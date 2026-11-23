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

func steamChart(ctx context.Context, year int, genre, region string) (annualChart, error) {
	salesContext, cancel := context.WithTimeout(ctx, 7*time.Second)
	best, err := steamMarket(salesContext, year, genre, region, true)
	cancel()
	if err == nil && len(best.Items) >= 10 {
		return best, nil
	}
	releases, fallbackErr := steamMarket(ctx, year, genre, region, false)
	if len(releases.Items) > 0 {
		return releases, nil
	}
	if len(best.Items) > 0 || err == nil {
		return best, nil
	}
	return releases, fallbackErr
}

func steamMarket(ctx context.Context, year int, genre, region string, topSellers bool) (annualChart, error) {
	chart := annualChart{Year: year, Source: "Steam", Region: region, Scope: "steam-market", Items: []entry{}}
	params := url.Values{"query": {""}, "start": {"0"}, "count": {"100"}, "sort_by": {"Released_DESC"}, "category1": {"998"}, "ndl": {"1"}, "infinite": {"1"}, "l": {"english"}, "cc": {region}}
	pages := 1
	if topSellers {
		chart.Scope = "steam-sales"
		params.Set("filter", "topsellers")
		params.Set("sort_by", "_ASC")
		pages = 3
	}
	if tag := steamChartTags[genre]; tag != "" {
		params.Set("tags", tag)
	}
	seen := map[string]bool{}
	for page := 0; page < pages && len(chart.Items) < 50; page++ {
		params.Set("start", strconv.Itoa(page*100))
		var payload struct {
			Results string `json:"results_html"`
		}
		if err := getJSON(ctx, "https://store.steampowered.com/search/results/?"+params.Encode(), "", &payload); err != nil {
			return chart, err
		}
		rows := steamSearchRows.FindAllStringSubmatch(payload.Results, -1)
		fresh := 0
		for _, row := range rows {
			if seen[row[1]] {
				continue
			}
			seen[row[1]] = true
			fresh++
			name, date := steamSearchName.FindStringSubmatch(row[2]), steamSearchDate.FindStringSubmatch(row[2])
			if len(name) == 0 || len(date) == 0 {
				continue
			}
			released, err := time.Parse("2 Jan, 2006", strings.TrimSpace(date[1]))
			if err != nil {
				released, err = time.Parse("Jan 2, 2006", strings.TrimSpace(date[1]))
			}
			if err != nil || released.Year() != year || released.After(time.Now()) {
				continue
			}
			var genres []string
			if genre != "" {
				genres = []string{genre}
			}
			chart.Items = append(chart.Items, entry{ID: "game:steam:" + row[1], Kind: "game", Title: html.UnescapeString(name[1]),
				Subtitle: released.Format(time.DateOnly), Cover: "https://cdn.cloudflare.steamstatic.com/steam/apps/" + row[1] + "/header.jpg", Genres: genres, Source: "Steam"})
			if len(chart.Items) == 50 {
				break
			}
		}
		if fresh == 0 {
			break
		}
	}

	return chart, nil
}
