package main

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var wikidataID = regexp.MustCompile(`^Q[0-9]+$`)

func searchGames(ctx context.Context, title, region string) ([]entry, error) {
	params := url.Values{"term": {title}, "cc": {region}, "l": {"german"}}
	var result struct {
		Items []struct {
			ID         int
			Name, Type string
			TinyImage  string `json:"tiny_image"`
		}
	}
	err := getJSON(ctx, "https://store.steampowered.com/api/storesearch/?"+params.Encode(), "", &result)
	items := []entry{}
	for _, game := range result.Items {
		if game.ID <= 0 || game.Name == "" || game.Type != "app" {
			continue
		}
		items = append(items, entry{ID: "game:steam:" + strconv.Itoa(game.ID), Kind: "game", Title: game.Name, Cover: game.TinyImage, Source: "Steam"})
	}
	return items, err
}

func steamGame(ctx context.Context, key, region string) (entry, error) {
	params := url.Values{"appids": {key}, "cc": {region}, "l": {"german"}}
	var result map[string]struct {
		Success bool
		Data    struct {
			Name, Type       string
			ShortDescription string `json:"short_description"`
			HeaderImage      string `json:"header_image"`
			Genres           []struct{ Description string }
			Developers       []string
		}
	}
	err := getJSON(ctx, "https://store.steampowered.com/api/appdetails?"+params.Encode(), "", &result)
	if err != nil {
		return entry{}, err
	}
	item := result[key]
	if !item.Success || item.Data.Name == "" {
		return entry{}, errors.New("Kein Steam-Titel")
	}
	genres := []string{}
	for _, genre := range item.Data.Genres {
		genres = append(genres, genre.Description)
	}
	return entry{ID: "game:steam:" + key, Kind: "game", Title: item.Data.Name, Subtitle: strings.Join(item.Data.Developers, ", "),
		Cover: item.Data.HeaderImage, Description: limitDescription(item.Data.ShortDescription), Genres: prettyGenres(genres), Source: "Steam"}, nil
}

func wikidataGame(ctx context.Context, key string) (entry, error) {
	params := url.Values{"action": {"wbgetentities"}, "format": {"json"}, "ids": {key}, "props": {"labels|descriptions"}, "languages": {"de|en"}}
	var result struct {
		Entities map[string]struct {
			Labels, Descriptions map[string]struct{ Value string }
		}
	}
	err := getJSON(ctx, "https://www.wikidata.org/w/api.php?"+params.Encode(), "", &result)
	data := result.Entities[key]
	name, description := data.Labels["de"].Value, data.Descriptions["de"].Value
	if name == "" {
		name = data.Labels["en"].Value
	}
	if description == "" {
		description = data.Descriptions["en"].Value
	}
	return entry{ID: "game:wikidata:" + key, Kind: "game", Title: name, Description: limitDescription(description), Source: "Wikidata"}, err
}
