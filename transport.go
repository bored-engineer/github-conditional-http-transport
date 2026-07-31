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

// CacheStatusHeader is set on every response per RFC 9211, e.g. `Cache-Status: github-conditional-http-transport; hit`.
const CacheStatusHeader = "Cache-Status"

// XCacheHeader is set on every response using the de facto X-Cache convention, e.g. `X-Cache: HIT`.
const XCacheHeader = "X-Cache"

// cacheName identifies this cache in the CacheStatusHeader, per RFC 9211.
const cacheName = "github-conditional-http-transport"

// CacheStatusValue/XCacheValue are set when the response was served from the cache.
const (
	CacheStatusValue = cacheName + `; hit`
	XCacheValue      = "HIT"
)

// XCacheMissValue/XCacheStoredValue/XCacheBypassValue are set on XCacheHeader when the response was not served from the cache.
const (
	XCacheMissValue   = "MISS"
	XCacheStoredValue = "MISS; stored"
	XCacheBypassValue = "BYPASS"
)

type transport struct {
	storage Storage
	parent  http.RoundTripper
}

// setCacheStatus sets the CacheStatusHeader/XCacheHeader pair on resp, initializing resp.Header if necessary.
func setCacheStatus(resp *http.Response, cacheStatus, xCache string) {
	if resp.Header == nil {
		resp.Header = make(http.Header)
	}
	resp.Header.Set(CacheStatusHeader, cacheStatus)
	resp.Header.Set(XCacheHeader, xCache)
}

// cacheStatusForward builds the CacheStatusHeader value for a request that was forwarded upstream
// (i.e. not a cache hit), recording the reason (per RFC 9211's fwd parameter), the resulting upstream
// status code (fwd-status), and whether the response was stored for future requests.
func cacheStatusForward(reason string, statusCode int, stored bool) string {
	v := fmt.Sprintf("%s; fwd=%s; fwd-status=%d", cacheName, reason, statusCode)
	if stored {
		v += "; stored"
	}
	return v
}

// RoundTrip implements the http.RoundTripper interface.
func (t *transport) RoundTrip(req *http.Request) (resp *http.Response, _ error) {
	// If the request is not cacheable, just pass it through to the parent RoundTripper
	if ok, reason := cacheable(req); !ok {
		resp, err := t.parent.RoundTrip(req)
		if err != nil {
			return nil, err
		}
		setCacheStatus(resp, cacheStatusForward(reason, resp.StatusCode, false), XCacheBypassValue)
		return resp, nil
	}

	// Attempt to fetch from storage
	cached, err := t.storage.Get(req.Context(), req)
	if err != nil {
		return nil, fmt.Errorf("(Storage).Get failed: %w", err)
	}
	defer func() {
		// If we did not utilize the cached response, ensure it is consumed and closed
		if cached != nil && cached.Body != nil && (resp == nil || resp.Body != cached.Body) {
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

	if resp.StatusCode == http.StatusNotModified {
		// If the upstream response is 304 Not Modified, we can use the cached response

		// Consume the rest of the response body to ensure the connection can be re-used
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			return nil, fmt.Errorf("(*http.Response).Body.Read failed: %w", err)
		}
		if err := resp.Body.Close(); err != nil {
			return nil, fmt.Errorf("(*http.Response).Body.Close failed: %w", err)
		}

		// Indicate the response was served from cache
		setCacheStatus(resp, CacheStatusValue, XCacheValue)

		// Copy in any cached headers that are not already set
		if cached != nil {
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
		} else {
			// Our speculative `[]` ETag guess matched the body
			resp.StatusCode = http.StatusOK
			resp.Status = http.StatusText(http.StatusOK)
		}

		// As a special case, if the request is a HEAD, we return an empty body
		if req.Method == http.MethodHead {
			resp.Body = io.NopCloser(strings.NewReader(""))
			resp.ContentLength = 0
		} else if cached != nil {
			resp.Body = cached.Body
			resp.ContentLength = cached.ContentLength
		} else {
			// We had no cached response, but our speculative `[]` ETag guess matched the body
			resp.Body = io.NopCloser(strings.NewReader("[]"))
			resp.ContentLength = 2
		}

	} else {
		stored := false

		if resp.StatusCode == http.StatusOK && req.Method == http.MethodGet && resp.Header.Get("Etag") != "" {
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
			if err := t.storage.Put(req.Context(), &cacheResp); err != nil {
				return resp, fmt.Errorf("(Storage).Put failed: %w", err)
			}
			stored = true

			// Restore the copied response body with the cached body
			resp.Body = cacheResp.Body
			resp.ContentLength = cacheResp.ContentLength
		}

		// The response was not served from the cache: if a cached response existed, it turned out to be
		// stale (revalidation failed); otherwise there was no candidate response to revalidate at all.
		reason := "uri-miss"
		if cached != nil {
			reason = "stale"
		}
		if stored {
			setCacheStatus(resp, cacheStatusForward(reason, resp.StatusCode, true), XCacheStoredValue)
		} else {
			setCacheStatus(resp, cacheStatusForward(reason, resp.StatusCode, false), XCacheMissValue)
		}
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
