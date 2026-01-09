package ghtransport

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"slices"
)

// addConditionalHeaders injects the conditional headers into the HTTP request if a cached response is available.
func addConditionalHeaders(req *http.Request, cached *http.Response) error {
	// If we have no cached response, bail, nothing to do
	if cached == nil {
		return nil
	}

	// If the Vary headers are all identical to the cached values, we can use the cached ETag directly
	if identicalVary(req, cached) {
		req.Header.Set("If-None-Match", cached.Header.Get("Etag"))
		return nil
	}

	// We'll have to consume the cached response body into memory to calculate the ETag
	var buf bytes.Buffer
	if cached.ContentLength > 0 {
		buf.Grow(int(cached.ContentLength))
	}
	if _, err := buf.ReadFrom(cached.Body); err != nil {
		return fmt.Errorf("(*http.Response).Body.Read failed: %w", err)
	}
	if err := cached.Body.Close(); err != nil {
		return fmt.Errorf("(*http.Response).Body.Close failed: %w", err)
	}
	cached.Body = io.NopCloser(&buf)
	cached.ContentLength = int64(buf.Len())

	// Calculate the _expected_ ETag from the _input_ headers but the cached body
	h := Hash(req.Header, slices.Collect(parseVary(cached.Header)))
	if _, err := h.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("(hash.Hash).Write failed: %w", err)
	}
	req.Header.Set("If-None-Match", `"`+hex.EncodeToString(h.Sum(nil))+`"`)

	return nil
}
