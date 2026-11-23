package main

import (
	"reflect"
	"testing"
)

func TestClassifyBookGenres(t *testing.T) {
	for _, test := range []struct {
		name   string
		groups [][]string
		want   []string
	}{
		{"specific before broad", [][]string{{"Fiktionale Darstellung", "Liebesroman"}, {"Dark Romance", "love stories", "Roman"}}, []string{"Dark Romance", "Liebesroman", "Belletristik", "Roman"}},
		{"format and theme", [][]string{{"Comics"}, {"Manga", "Science fiction -- Juvenile fiction"}}, []string{"Manga", "Comic", "Sci-Fi"}},
		{"unrelated headings", [][]string{{"History of textbook design", "Schweden", "(Produktform)Taschenbuch"}}, []string{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := classifyBookGenres(test.groups...); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("classifyBookGenres = %v, want %v", got, test.want)
			}
		})
	}
}
