package ghtransport

import (
	"fmt"
	"io"
	"maps"
	"net/http"
	"strings"
)

// CachedRequestIDHeader is the X-Github-Request-Id header from the cached response.
const CachedRequestIDHeader = "X-Cached-Request-Id"

type transport struct {
	storage Storage
	parent  http.RoundTripper
}

// RoundTrip implements the http.RoundTripper interface.
func (t *transport) RoundTrip(req *http.Request) (resp *http.Response, _ error) {
	// If the request is not cacheable, just pass it through to the parent RoundTripper
	if !cacheable(req) {
		return t.parent.RoundTrip(req)
	}

	// Attempt to fetch from storage
	cached, err := t.storage.Get(req.Context(), req.URL)
	if err != nil {
		return nil, fmt.Errorf("(Storage).Get failed: %w", err)
	}
	defer func() {
		// If we did not utilize the cached response, ensure it is consumed and closed
		if cached != nil && cached.Body != nil && resp.Body != cached.Body {
			_, _ = io.Copy(io.Discard, cached.Body)
			_ = cached.Body.Close()
		}
	}()

	// Per the http.RoundTripper contract, we cannot modify the request in-place, we need to shallow clone it
	req = req.Clone(req.Context())

	// If there is a User-Agent, ensure it's compatible
	replaceUserAgent(req.Header)

	// Inject the conditional headers to the request
	if err := addConditionalHeaders(req, cached); err != nil {
		return nil, fmt.Errorf("failed to inject conditional headers: %w", err)
	}

	// Perform the upstream request
	resp, err = t.parent.RoundTrip(req)
	if err != nil {
		return nil, fmt.Errorf("(http.RoundTripper).RoundTrip failed: %w", err)
	}

	if resp.StatusCode == http.StatusNotModified && cached != nil {
		// If the upstream response is 304 Not Modified, we can use the cached response

		// Consume the rest of the response body to ensure the connection can be re-used
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			return nil, fmt.Errorf("(*http.Response).Body.Read failed: %w", err)
		}
		if err := resp.Body.Close(); err != nil {
			return nil, fmt.Errorf("(*http.Response).Body.Close failed: %w", err)
		}

		// Copy in any cached headers that are not already set
		for key, vals := range cached.Header {
			if strings.HasPrefix(key, VaryPrefix) {
				continue // Skip the X-Varied-* headers, they are "internal" to the cache
			}
			if key == "X-Github-Request-Id" {
				// Return the original Request-Id header as well
				resp.Header[CachedRequestIDHeader] = vals
			}
			if _, ok := resp.Header[key]; !ok {
				resp.Header[key] = vals
			}
		}

		// Copy the body and status from the cache
		resp.StatusCode = cached.StatusCode
		resp.Status = cached.Status
		resp.Body = cached.Body
		resp.ContentLength = cached.ContentLength

	} else if resp.StatusCode == http.StatusOK && resp.Header.Get("Etag") != "" {
		// Make a shallow copy of the *http.Response as we're going to modify the headers for storage
		cacheResp := *resp
		cacheResp.Header = maps.Clone(resp.Header)

		// Inject fake X-Varied-<header> "response" headers
		for header := range parseVary(resp.Header) {
			header = http.CanonicalHeaderKey(header)
			if vals := req.Header.Values(header); len(vals) > 0 {
				if header == "Authorization" {
					vals = []string{HashToken(vals[0])} // Don't leak/cache the raw authentication token
				}
				cacheResp.Header[VaryPrefix+header] = vals
			}
		}

		// Store the cached response body as bytes
		// Per the storage contract, they will restore the Body/ContentLength after consumption
		if err := t.storage.Put(req.Context(), req.URL, &cacheResp); err != nil {
			return resp, fmt.Errorf("(Storage).Put failed: %w", err)
		}

		// Restore the copied response body with the cached body
		resp.Body = cacheResp.Body
		resp.ContentLength = cacheResp.ContentLength
	}

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
