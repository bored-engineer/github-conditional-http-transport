package ghtransport

import (
	"net/http"
	"strings"
)

// Responses from the GitHub REST API are pretty-printed if the User-Agent contains "curl", "Wget", "Safari" or "Firefox".
// This breaks the ETag calculation, so we need to substitute these strings if present in the User-Agent.
var UserAgentReplacer = strings.NewReplacer(
	"curl", "cUrL",
	"Wget", "wGeT",
	"Safari", "sAfArI",
	"Firefox", "fIrEfOx",
)

func replaceUserAgent(headers http.Header) {
	if ua := headers.Get("User-Agent"); ua != "" {
		headers.Set("User-Agent", UserAgentReplacer.Replace(ua))
	}
}
