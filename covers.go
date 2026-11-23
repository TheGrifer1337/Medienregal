package main

import (
	"encoding/base64"
	"image/jpeg"
	"net/url"
	"strings"
)

const embeddedCoverPrefix = "data:image/jpeg;base64,"

func validCustomCover(value string) bool {
	if value == "" {
		return true
	}
	if strings.HasPrefix(value, embeddedCoverPrefix) {
		if len(value) > 330000 {
			return false
		}
		data := base64.NewDecoder(base64.StdEncoding, strings.NewReader(strings.TrimPrefix(value, embeddedCoverPrefix)))
		image, err := jpeg.DecodeConfig(data)
		return err == nil && image.Width > 0 && image.Width <= 500 && image.Height > 0 && image.Height <= 750
	}
	address, err := url.Parse(value)
	return err == nil && len(value) <= 4096 && address.Scheme == "https" && address.Hostname() != "" && address.User == nil
}
