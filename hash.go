package ghtransport

import (
	"crypto/sha256"
	"hash"
	"net/http"
)

// VaryHeaders are the headers that are used to vary the cache key, this slice _must_ remain sorted.
var VaryHeaders = []string{
	"Accept",
	"Authorization",
	"Cookie",
}

// Hash initializes a hash.Hash following the GitHub's internal ETag implementation.
// The response body must be written to the hash before it can be used to calculate the ETag.
func Hash(requestHeaders http.Header) hash.Hash {
	h := sha256.New()
	for _, headerName := range VaryHeaders {
		for _, headerValue := range requestHeaders.Values(headerName) {
			h.Write(append([]byte(headerValue), ':'))
		}
	}
	return h
}
