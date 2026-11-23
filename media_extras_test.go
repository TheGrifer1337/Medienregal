package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/oned"
)

func TestBarcodeScansAnISBN(t *testing.T) {
	const isbn = "9780140328721"
	matrix, err := oned.NewEAN13Writer().Encode(isbn, gozxing.BarcodeFormat_EAN_13, 340, 120, nil)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewGray(image.Rect(0, 0, matrix.GetWidth(), matrix.GetHeight()))
	for y := 0; y < matrix.GetHeight(); y++ {
		for x := 0; x < matrix.GetWidth(); x++ {
			if !matrix.Get(x, y) {
				img.SetGray(x, y, color.Gray{Y: 255})
			}
		}
	}
	var picture bytes.Buffer
	if err := png.Encode(&picture, img); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	(&app{}).routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/barcode", &picture))
	var found struct{ Code string }
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &found) != nil || found.Code != isbn {
		t.Fatalf("unexpected scan: %d %s", response.Code, response.Body.String())
	}
	if validGTIN("9780140328722") || !validGTIN("036000291452") {
		t.Fatal("bad GTIN check digit validation")
	}
}

func TestExtrasAndGoalsPersist(t *testing.T) {
	file := filepath.Join(t.TempDir(), "library.json")
	app := &app{path: file}
	request := func(method, endpoint, data string) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		app.routes().ServeHTTP(recorder, httptest.NewRequest(method, endpoint, strings.NewReader(data)))
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", endpoint, recorder.Code, recorder.Body.String())
		}
		return recorder
	}
	request("PUT", "/api/settings", `{"region":"JP","language":"zh-Hans"}`)
	request("PUT", "/api/goals", `{"year":2026,"targets":{"movie":10}}`)
	request("PUT", "/api/goals", `{"year":2027,"targets":{"book":3}}`)
	request("PUT", "/api/entries", `{"id":"manual:film","kind":"movie","title":"Part II","status":"seen","series":"Example","sequence":2.5,"loan":{"to":"Friend","since":"2026-11-01","due":"2026-12-01"},"trailer":"dQw4w9WgXcQ"}`)
	data, err := readLibrary(file)
	if err != nil || data.Settings.locale() != "zh-Hans" || data.Settings.Region != "JP" || data.Goals[2026]["movie"] != 10 || data.Goals[2027]["book"] != 3 ||
		len(data.Entries) != 1 || data.Entries[0].Loan == nil || data.Entries[0].Sequence != 2.5 || data.Entries[0].Trailer != "dQw4w9WgXcQ" || data.Entries[0].CompletedYear != time.Now().Year() {
		t.Fatalf("unexpected library %+v, error: %v", data, err)
	}
	request("PUT", "/api/goals", `{"year":2026,"targets":{"movie":0}}`)
	if app.data.Goals[2026] != nil || app.data.Goals[2027]["book"] != 3 {
		t.Fatalf("goal deletion removed unrelated data: %v", app.data.Goals)
	}
}

func TestRegionalMovieChartAndTrailer(t *testing.T) {
	original := webClient
	t.Cleanup(func() { webClient = original })
	webClient = &http.Client{Transport: coverTestTransport(func(r *http.Request) (*http.Response, error) {
		var body string
		switch r.URL.Hostname() {
		case "api.imdbapi.dev":
			if r.URL.Query().Get("countryCodes") != "JP" {
				return nil, fmt.Errorf("market missing: %s", r.URL)
			}
			body = fmt.Sprintf(`{"titles":[{"id":"tt1234","primaryTitle":"Japanese example","startYear":%d,"originCountries":[{"code":"JP"}],"rating":{"aggregateRating":8.1}},{"id":"tt5678","primaryTitle":"Wrong market","startYear":%d,"originCountries":[{"code":"US"}]}]}`, time.Now().Year(), time.Now().Year())
		case "v3-cinemeta.strem.io":
			body = `{"meta":{"imdbRating":"8.2","trailers":[{"type":"Trailer","source":"dQw4w9WgXcQ"}]}}`
		default:
			return nil, fmt.Errorf("unexpected request: %s", r.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	handler := (&app{data: library{Settings: settings{Region: "JP", Language: "zh-Hans"}}}).routes()
	chart := httptest.NewRecorder()
	handler.ServeHTTP(chart, httptest.NewRequest(http.MethodGet, "/api/rankings?kind=movie", nil))
	var result annualChart
	if chart.Code != http.StatusOK || json.Unmarshal(chart.Body.Bytes(), &result) != nil || result.Region != "JP" ||
		len(result.Items) != 1 || result.Items[0].IMDb != 8.1 || result.Items[0].ID != "movie:imdb:tt1234" {
		t.Fatalf("unexpected regional chart: %d %s", chart.Code, chart.Body.String())
	}
	for endpoint, expected := range map[string]string{"imdb-rating": `"score":8.2`, "movie-trailer": `"trailer":"dQw4w9WgXcQ"`} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/"+endpoint+"?id=movie:imdb:tt1234", nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), expected) {
			t.Fatalf("%s returned %s", endpoint, response.Body.String())
		}
	}
}
