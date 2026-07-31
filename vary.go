package ghtransport

import (
	"iter"
	"net/http"
	"slices"
	"strings"
)

// VaryPrefix is used to store the "Vary" header values from the _request_ as fake "response" headers.
var VaryPrefix = "X-Varied-"

// identicalVary checks if the Vary headers are all identical to the cached values.
func identicalVary(req *http.Request, cached *http.Response) bool {
	for header := range parseVary(cached.Header) {
		switch header {
		case "*":
			// Per RFC 9110 12.5.5, "Vary: *" means the representation may vary on
			// unspecified request headers, so it can never be treated as identical.
			return false
		case "Authorization":
			// Special case, we need to hash the Authorization header before comparing
			if HashToken(req.Header.Get("Authorization")) != cached.Header.Get(VaryPrefix+header) {
				return false
			}
		default:
			// Compare all values (not just the first), matching how they're stored in transport.go
			if !slices.Equal(req.Header.Values(header), cached.Header.Values(VaryPrefix+header)) {
				return false
			}
		}
	}
	return true
}

// parseVary parses the 'Vary' header (comma-separated) into a iterable of strings.
func parseVary(headers http.Header) iter.Seq[string] {
	return func(yield func(string) bool) {
		for _, val := range headers.Values("Vary") {
			for field := range strings.FieldsFuncSeq(val, func(r rune) bool {
				return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
			}) {
				if !yield(field) {
					return
				}
			}
		}
	}
}
