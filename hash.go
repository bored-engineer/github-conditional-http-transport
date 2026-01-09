package ghtransport

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"hash"
	"net/http"
	"slices"
	"strings"
)

// VaryHeaders are the headers that are used to vary the cache key, this slice _must_ remain sorted.
var VaryHeaders = []string{
	"Accept",
	"Authorization",
	"Cookie",
}

// Hash initializes a hash.Hash following the GitHub's internal ETag implementation.
// The response body must be written to the hash before it can be used to calculate the ETag.
func Hash(requestHeaders http.Header, vary []string) hash.Hash {
	h := sha256.New()
	for _, headerName := range VaryHeaders {
		if vary == nil || slices.Contains(vary, headerName) {
			for _, headerValue := range requestHeaders.Values(headerName) {
				h.Write(append([]byte(headerValue), ':'))
			}
		}
	}
	return h
}

// HashToken returns a hash of the 'Authorization' header matching the 'hashed_token' audit log field from GitHub.
// https://docs.github.com/en/enterprise-cloud@latest/admin/monitoring-activity-in-your-enterprise/reviewing-audit-logs-for-your-enterprise/identifying-audit-log-events-performed-by-an-access-token
func HashToken(authorization string) string {
	// If the authorization header is empty, we hash an empty string
	var token string
	// This is the most common pattern
	if bearer, ok := strings.CutPrefix(authorization, "Bearer "); ok && bearer != "" {
		token = bearer
	}
	// This is the second most common pattern
	if basic, ok := strings.CutPrefix(authorization, "Basic "); ok && basic != "" {
		if decoded, err := base64.StdEncoding.DecodeString(basic); err == nil {
			if _, password, ok := bytes.Cut(decoded, []byte{':'}); ok && len(password) > 0 {
				token = string(password)
			}
		}
	}
	// This is also supported but not very common
	if bearer, ok := strings.CutPrefix(authorization, "token "); ok && bearer != "" {
		token = bearer
	}
	hashed := sha256.Sum256([]byte(token))
	return base64.StdEncoding.EncodeToString(hashed[:])
}
