package main

import (
	"bytes"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/oned"
)

var gtin = regexp.MustCompile(`^(?:[0-9]{8}|[0-9]{12,14})$`)

func validGTIN(code string) bool {
	if !gtin.MatchString(code) {
		return false
	}
	sum, factor := 0, 1
	for i := len(code) - 1; i >= 0; i-- {
		sum += int(code[i]-'0') * factor
		factor = 4 - factor
	}
	return sum%10 == 0
}

func (a *app) scanBarcode(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 3*1024*1024))
	if err != nil {
		fail(w, 400, "Das Bild ist zu groß.")
		return
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 2400 || config.Height > 2400 {
		fail(w, 400, "Das Bild konnte nicht gelesen werden.")
		return
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		fail(w, 400, "Das Bild konnte nicht gelesen werden.")
		return
	}
	bitmap, err := gozxing.NewBinaryBitmapFromImage(img)
	if err == nil {
		hints := map[gozxing.DecodeHintType]interface{}{
			gozxing.DecodeHintType_TRY_HARDER: true,
			gozxing.DecodeHintType_POSSIBLE_FORMATS: []gozxing.BarcodeFormat{
				gozxing.BarcodeFormat_EAN_13, gozxing.BarcodeFormat_EAN_8, gozxing.BarcodeFormat_UPC_A,
			},
		}
		result, err := oned.NewMultiFormatUPCEANReader(hints).Decode(bitmap, hints)
		if err == nil && validGTIN(result.GetText()) {
			reply(w, 200, map[string]string{"code": result.GetText()})
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) barcodeLookup(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if !validGTIN(code) {
		fail(w, 400, "Gib einen gültigen EAN- oder UPC-Code ein.")
		return
	}
	var lookup struct {
		Items []struct{ Title string }
	}
	err := getJSON(r.Context(), "https://api.upcitemdb.com/prod/trial/lookup?"+url.Values{"upc": {code}}.Encode(), "", &lookup)
	if err != nil || len(lookup.Items) == 0 || strings.TrimSpace(lookup.Items[0].Title) == "" {
		fail(w, 404, "Für diesen Barcode wurde kein Produkt gefunden. Suche stattdessen nach dem Titel auf der Hülle.")
		return
	}
	title := strings.TrimSpace(lookup.Items[0].Title)
	if len(title) > 600 {
		fail(w, 404, "Für diesen Barcode wurde kein Produkt gefunden. Suche stattdessen nach dem Titel auf der Hülle.")
		return
	}
	reply(w, 200, map[string]string{"title": title})
}
