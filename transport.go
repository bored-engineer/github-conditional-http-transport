package ghtransport

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strings"
)

// Responses from the GitHub REST API are pretty-printed if the User-Agent contains "curl", "Wget", "Safari" or "Firefox".
// This breaks the ETag calculation, so we need to replace the User-Agent with a "fake" User-Agent that doesn't contain these strings.
var uaReplacer = strings.NewReplacer(
	"curl", "cUrL",
	"Wget", "wGeT",
	"Safari", "sAfArI",
	"Firefox", "fIrEfOx",
)

// cacheable determines if the request is potentially cacheable.
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

// parseCommaSepHeader parses a comma-separated header into a slice of strings.
func parseCommaSepHeader(headers http.Header, name string) (vals []string) {
	for _, val := range headers.Values(name) {
		vals = append(vals, strings.FieldsFunc(val, func(r rune) bool {
			return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
		})...)
	}
	return
}

type transport struct {
	storage Storage
	parent  http.RoundTripper
}

func (t *transport) injectCacheHeaders(req *http.Request) (resp *http.Response, err error) {
	// Check if we have a cached response available in the storage for this URL, else bail
	resp, err = t.storage.Get(req.Context(), req.URL)
	if err != nil {
		return nil, fmt.Errorf("(Storage).Get failed: %w", err)
	} else if resp == nil {
		return nil, nil
	}
	defer func() {
		// If we're not using the cached response, ensure we close the body
		// But first, read it to completion to ensure the connection can be re-used
		if resp == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
	}()

	// If we're using the same hashed_token, we can directly use the cached etag
	if HashToken(req.Header.Get("Authorization")) == resp.Header.Get("X-Varied-Authorization") {
		req.Header.Set("If-None-Match", resp.Header.Get("Etag"))
		return resp, nil
	}

	// We'll have to read the cached response body into memory to calculate the ETag
	var buf bytes.Buffer

	// Calculate the _expected_ ETag from the _input_ headers but the cached body
	h := Hash(req.Header)
	if _, err := io.Copy(io.MultiWriter(&buf, h), resp.Body); err != nil {
		return nil, fmt.Errorf("(*http.Response).Body.Read failed: %w", err)
	}
	if err := resp.Body.Close(); err != nil {
		return nil, fmt.Errorf("(*http.Response).Body.Close failed: %w", err)
	}

	// Add the If-None-Match header to the request with that calculated ETag
	req.Header.Set("If-None-Match", `"`+hex.EncodeToString(h.Sum(nil))+`"`)

	// Make the next "read" from the cached body use the bytes we just read
	resp.Body = io.NopCloser(&buf)
	resp.ContentLength = int64(buf.Len())

	return resp, nil
}

// RoundTrip implements the http.RoundTripper interface.
func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	// If the request is not cacheable, just pass it through to the parent RoundTripper
	if !cacheable(req) {
		return t.parent.RoundTrip(req)
	}

	// If there is a User-Agent, ensure it's compatible
	if ua := req.Header.Get("User-Agent"); ua != "" {
		req.Header.Set("User-Agent", uaReplacer.Replace(ua))
	}

	// Attempt to fetch from storage and inject the cache headers to the request
	cachedResp, err := t.injectCacheHeaders(req)
	if err != nil {
		return nil, err
	}

	// Perform the upstream request
	resp, err := t.parent.RoundTrip(req)
	if err != nil {
		if cachedResp != nil {
			cachedResp.Body.Close()
		}
		return nil, err
	}

	// If the upstream response is 304 Not Modified, we can use the cached response
	if cachedResp != nil {
		if resp.StatusCode == http.StatusNotModified {
			// Consume the rest of the response body to ensure the connection can be re-used
			if _, err := io.Copy(io.Discard, resp.Body); err != nil {
				cachedResp.Body.Close()
				return nil, fmt.Errorf("(*http.Response).Body.Read failed: %w", err)
			}
			if err := resp.Body.Close(); err != nil {
				cachedResp.Body.Close()
				return nil, fmt.Errorf("(*http.Response).Body.Close failed: %w", err)
			}

			// Copy in any cached headers that are not already set
			for key, vals := range cachedResp.Header {
				if strings.HasPrefix(key, "X-Varied-") {
					continue // Skip the X-Varied-* headers, they are "internal" to the cache
				}
				if key == "X-Github-Request-Id" {
					// Return the original Request-Id header as well
					resp.Header["X-Cached-Request-Id"] = vals
				}
				if _, ok := resp.Header[key]; !ok {
					resp.Header[key] = vals
				}
			}

			// Copy the body and status from the cache
			resp.StatusCode = cachedResp.StatusCode
			resp.Status = cachedResp.Status
			resp.Body = cachedResp.Body
			resp.ContentLength = cachedResp.ContentLength

			return resp, nil
		} else {
			// Discard the cached response body, it wasn't valid/used
			_, _ = io.Copy(io.Discard, cachedResp.Body)
			_ = cachedResp.Body.Close()
		}
	}

	// We got a valid response, try to cache it
	resp, err = t.cacheResponse(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (t *transport) cacheResponse(req *http.Request, resp *http.Response) (*http.Response, error) {
	// We can only cache successful responses
	if resp.StatusCode != http.StatusOK {
		return resp, nil
	}

	// If there was no ETag, we can't cache it
	if resp.Header.Get("Etag") == "" {
		return resp, nil
	}

	// Read the response body into memory
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, fmt.Errorf("(*http.Response).Body.Read failed: %w", err)
	}
	if err := resp.Body.Close(); err != nil {
		return resp, fmt.Errorf("(*http.Response).Body.Close failed: %w", err)
	}

	// Make a shallow copy of the *http.Response as we're going to modify the body/headers
	cacheResp := *resp
	cacheResp.Body = io.NopCloser(bytes.NewReader(body))
	cacheResp.ContentLength = int64(len(body))
	cacheResp.Header = maps.Clone(resp.Header)

	// Similar to httpcache, inject fake X-Varied-<header> "response" headers
	for _, header := range parseCommaSepHeader(resp.Header, "Vary") {
		header = http.CanonicalHeaderKey(header)
		if vals := req.Header.Values(header); len(vals) > 0 {
			if header == "Authorization" {
				vals = []string{HashToken(vals[0])} // Don't leak/cache the raw authentication token
			}
			cacheResp.Header["X-Varied-"+header] = vals
		}
	}

	if err := t.storage.Put(req.Context(), req.URL, &cacheResp); err != nil {
		return resp, fmt.Errorf("(Storage).Put failed: %w", err)
	}

	// Replace the response body with the cached body
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	return resp, nil
}

// NewTransport creates a new http.RoundTripper that reads/writes responses from the Storage.
func NewTransport(storage Storage, parent http.RoundTripper) http.RoundTripper {
	if parent == nil {
		parent = http.DefaultTransport
	}
	return &transport{
		storage: storage,
		parent:  parent,
	}
}
