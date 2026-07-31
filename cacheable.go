package ghtransport

import "net/http"

// cacheable determines if a GitHub REST API request will likely be cacheable.
// If not cacheable, it also returns the RFC 9211 forwarding reason ("method" or "bypass").
func cacheable(req *http.Request) (bool, string) {
	if req.Method != "GET" && req.Method != "HEAD" {
		return false, "method"
	}
	if req.Header.Get("Range") != "" {
		return false, "bypass"
	}
	if req.URL.Path == "/rate_limit" || req.URL.Path == "/api/v3/rate_limit" {
		return false, "bypass"
	}
	return true, ""
}
