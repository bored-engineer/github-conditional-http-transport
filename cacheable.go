package ghtransport

import "net/http"

// cacheable determines if a GitHub REST API request will likely be cacheable.
func cacheable(req *http.Request) bool {
	if req.Method != "GET" && req.Method != "HEAD" {
		return false
	}
	if req.Header.Get("Range") != "" {
		return false
	}
	if req.URL.Path == "/rate_limit" || req.URL.Path == "/api/v3/rate_limit" {
		return false
	}
	return true
}
