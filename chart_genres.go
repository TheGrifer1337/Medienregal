package main

type chartGenre struct{ term, tmdb, wikidata string }

var movieGenres = map[string]chartGenre{
	"Abenteuer":     {"Adventure", "12", "wd:Q319221"},
	"Action":        {"Action", "28", "wd:Q188473"},
	"Animation":     {"Animation", "16", "wd:Q202866"},
	"Dokumentation": {"Documentary", "99", "wd:Q93204"},
	"Drama":         {"Drama", "18", "wd:Q130232"},
	"Fantasy":       {"Fantasy", "14", "wd:Q157394"},
	"Horror":        {"Horror", "27", "wd:Q200092"},
	"Komödie":       {"Comedy", "35", "wd:Q157443"},
	"Krimi":         {"Crime", "80", "wd:Q959790"},
	"Mystery":       {"Mystery", "9648", "wd:Q1200678"},
	"Romantik":      {"Romance", "10749", "wd:Q1054574"},
	"Sci-Fi":        {"Sci-Fi", "878", "wd:Q471839"},
	"Thriller":      {"Thriller", "53", "wd:Q2484376"},
}

var seriesGenres = map[string]chartGenre{
	"Abenteuer":     {"Adventure", "", "wd:Q319221"},
	"Action":        {"Action", "", "wd:Q188473"},
	"Animation":     {"Animation", "16", "wd:Q202866"},
	"Dokumentation": {"Documentary", "99", "wd:Q93204"},
	"Drama":         {"Drama", "18", "wd:Q130232"},
	"Fantasy":       {"Fantasy", "", "wd:Q157394"},
	"Horror":        {"Horror", "", "wd:Q200092"},
	"Komödie":       {"Comedy", "35", "wd:Q157443 wd:Q170238"},
	"Krimi":         {"Crime", "80", "wd:Q959790"},
	"Mystery":       {"Mystery", "9648", "wd:Q1200678"},
	"Romantik":      {"Romance", "", "wd:Q1054574"},
	"Sci-Fi":        {"Sci-Fi", "", "wd:Q471839"},
	"Thriller":      {"Thriller", "", "wd:Q2484376"},
}

var chartGenres = map[string]map[string]chartGenre{
	"movie":  movieGenres,
	"series": seriesGenres,
	"book": {
		"Fantasy": {"Fantasy", "", ""}, "Horror": {"Horror", "", ""}, "Krimi": {"Mystery", "", ""},
		"Liebesroman": {"Romance", "", ""}, "Manga": {"Manga", "", ""}, "Sci-Fi": {"Science fiction", "", ""},
		"Thriller": {"Thrillers", "", ""}, "Roman": {"Fiction", "", ""},
	},
	"game": {
		"Abenteuer": {"Adventure", "", "wd:Q343568"}, "Action": {"Action", "", "wd:Q270948"},
		"Horror":       {"Horror", "", "wd:Q193430 wd:Q333967"},
		"Jump ’n’ Run": {"Platformer", "", "wd:Q828322"},
		"Rennspiel":    {"Racing", "", "wd:Q860750"}, "Rollenspiel": {"RPG", "", "wd:Q744038"},
		"Simulation": {"Simulation", "", "wd:Q1610017"}, "Strategie": {"Strategy", "", "wd:Q472643"},
	},
}

func chartGenreOptions() map[string][]string {
	options := make(map[string][]string, len(chartGenres))
	for kind, genres := range chartGenres {
		for name := range genres {
			options[kind] = append(options[kind], name)
		}
	}
	return options
}
