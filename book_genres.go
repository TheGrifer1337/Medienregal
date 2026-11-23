package main

import (
	"context"
	"regexp"
	"sort"
	"strings"
)

var bookGenreRules = []struct {
	name  string
	terms string
}{
	{"Dark Romance", "dark romance|dark romantic fiction|dark liebesroman"},
	{"Romantasy", "romantasy|romantic fantasy|fantasy romance|fantasy liebesroman"},
	{"Manga", "manga|mangas|manga comics"},
	{"Horror", "horror|horror fiction|horror tales|horrorroman|horrorromane|gruselliteratur|gruselroman"},
	{"Sci-Fi", "science fiction|sci fi|sciencefiction|science fiction novels"},
	{"Fantasy", "fantasy|fantasy fiction|fantasy novels|fantasyroman|fantasyromane|fantastische literatur|epic fantasy|juvenile fantasy"},
	{"Dystopie", "dystopie|dystopien|dystopia|dystopias|dystopian fiction"},
	{"Thriller", "thriller|thrillers|thriller fiction|psychothriller|spannungsroman"},
	{"Krimi", "krimi|krimis|kriminalroman|kriminalromane|kriminalgeschichte|detective and mystery stories|crime fiction|crime novels"},
	{"Mystery", "mystery|mystery fiction|mystery novels"},
	{"Historischer Roman", "historischer roman|historische romane|historisches erzählwerk|historical fiction|historical novels"},
	{"Liebesroman", "liebesroman|liebesromane|romantik|romance|romance fiction|romance novels|love stories|romantic fiction"},
	{"Abenteuer", "abenteuer|abenteuerroman|abenteuerromane|adventure fiction|adventure stories"},
	{"Komödie", "komödie|comedy|humorous fiction|humorous stories"},
	{"Humor & Satire", "humor und satire|humor satire|satire|satires"},
	{"Graphic Novel", "graphic novel|graphic novels"},
	{"Comic", "comic|comics|comic books|comic strips"},
	{"Biografie", "biografie|biographien|biografie biographien|biographie|biografien|biography|biographies"},
	{"Autobiografie", "autobiografie|autobiographie|autobiography|autobiographies|memoir|memoirs"},
	{"Ratgeber", "ratgeber|self help|self help books|advice manuals"},
	{"Kochbuch", "kochbuch|kochbücher|cookbook|cookbooks"},
	{"Reisebuch", "reisebuch|reiseführer|guidebooks|travel guides"},
	{"Sachbuch", "sachbuch|sachbücher|nonfiction|non fiction|popular works"},
	{"Fachbuch", "fachbuch|fachbücher|textbook|textbooks|lehrbuch|lehrbücher"},
	{"Lyrik", "lyrik|gedichte|gedicht|poetry|poems"},
	{"Kurzgeschichten", "kurzgeschichte|kurzgeschichten|short stories"},
	{"Drama", "drama|theaterstück|plays"},
	{"Bilderbuch", "bilderbuch|bilderbücher|picture books"},
	{"Kinderbuch", "kinderbuch|kinderbücher|children s fiction|children s literature|children s books"},
	{"Jugendbuch", "jugendbuch|jugendbücher|young adult fiction|young adult literature|young adult novels"},
	{"Kinder- und Jugendbuch", "kinder und jugendliteratur|kinder und jugendbuch"},
	{"Roman", "roman|romane|novel|novels"},
	{"Belletristik", "belletristik|fiction|fiktionale darstellung"},
}

var bookTermSeparator = regexp.MustCompile(`[^\p{L}\p{N}]+`)
var openLibraryWork = regexp.MustCompile(`^/works/OL[0-9]+W$`)

var bookGenreTerms = func() map[string]string {
	terms := make(map[string]string)
	for _, rule := range bookGenreRules {
		for _, term := range strings.Split(rule.terms, "|") {
			terms[term] = rule.name
		}
	}
	return terms
}()

func classifyBookGenres(groups ...[]string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, group := range groups {
		for _, source := range group {
			// Catalog headings can carry subdivisions after a double dash.
			source = strings.SplitN(source, "--", 2)[0]
			term := strings.TrimSpace(bookTermSeparator.ReplaceAllString(strings.ToLower(source), " "))
			if name := bookGenreTerms[term]; name != "" && !seen[name] {
				result = append(result, name)
				seen[name] = true
			}
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		return bookGenrePriority(result[i]) > bookGenrePriority(result[j])
	})
	return result
}

func bookGenrePriority(name string) int {
	switch name {
	case "Dark Romance", "Romantasy", "Manga":
		return 2
	case "Belletristik", "Roman", "Kinder- und Jugendbuch", "Kinderbuch", "Jugendbuch":
		return 0
	default:
		return 1
	}
}

func openWorkGenres(ctx context.Context, key string) []string {
	if !openLibraryWork.MatchString(key) {
		return nil
	}
	var work struct{ Subjects []string }
	if err := getJSON(ctx, "https://openlibrary.org"+key+".json", "", &work); err != nil {
		return nil
	}
	return classifyBookGenres(work.Subjects)
}

func editionGenres(ctx context.Context, edition bookEdition) []string {
	genres := classifyBookGenres(edition.Subjects)
	if len(edition.Works) > 0 {
		return classifyBookGenres(genres, openWorkGenres(ctx, edition.Works[0].Key))
	}
	return genres
}
