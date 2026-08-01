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

// CacheName identifies this cache in the "Cache-Status" header, per RFC 9211. It may be overridden
// (e.g. by an application embedding this transport under its own name) before use.
var CacheName = "github-conditional-http-transport"

// cacheStatusHit builds the "Cache-Status" header value for a cache hit.
func cacheStatusHit() string {
	return CacheName + `; hit`
}

// cacheStatusHitSpeculative builds the "Cache-Status" header value for a "hit" that was the result of a
// speculative `[]` ETag guess (i.e. no response was ever actually stored for this request), per RFC
// 9211's optional "detail" parameter for conveying implementation-specific information.
func cacheStatusHitSpeculative() string {
	return cacheStatusHit() + `; detail=speculative-empty-array`
}

type transport struct {
	storage Storage
	parent  http.RoundTripper
}

// setCacheStatus sets the "Cache-Status"/"X-Cache" header pair on resp, initializing resp.Header if necessary.
func setCacheStatus(resp *http.Response, cacheStatus, xCache string) {
	if resp.Header == nil {
		resp.Header = make(http.Header)
	}
	resp.Header.Set("Cache-Status", cacheStatus)
	resp.Header.Set("X-Cache", xCache)
}

// cacheStatusForward builds the "Cache-Status" header value for a request that was forwarded upstream
// (i.e. not a cache hit), recording the reason (per RFC 9211's fwd parameter), the resulting upstream
// status code (fwd-status), and whether the response was stored for future requests.
func cacheStatusForward(reason string, statusCode int, stored bool) string {
	v := fmt.Sprintf("%s; fwd=%s; fwd-status=%d", CacheName, reason, statusCode)
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
		setCacheStatus(resp, cacheStatusForward(reason, resp.StatusCode, false), "MISS")
		return resp, nil
	}

	// Attempt to fetch from storage, if one is configured
	var cached *http.Response
	var err error
	if t.storage != nil {
		cached, err = t.storage.Get(req.Context(), req)
		if err != nil {
			return nil, fmt.Errorf("(Storage).Get failed: %w", err)
		}
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
		if cached != nil {
			setCacheStatus(resp, cacheStatusHit(), "HIT")
		} else {
			// Our speculative `[]` ETag guess matched; nothing was ever actually stored for this request
			setCacheStatus(resp, cacheStatusHitSpeculative(), "HIT")
		}

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

		if t.storage != nil && resp.StatusCode == http.StatusOK && req.Method == http.MethodGet && resp.Header.Get("Etag") != "" {
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
		setCacheStatus(resp, cacheStatusForward(reason, resp.StatusCode, stored), "MISS")
	}

	return resp, nil
}

// NewTransport creates a new http.RoundTripper that reads/writes responses from the Storage.
// A nil storage is safe to pass: nothing is ever read from or written to it, but the speculative
// empty-array ETag guess (see addConditionalHeaders) still applies to every cacheable request.
func NewTransport(storage Storage, parent http.RoundTripper) http.RoundTripper {
	if parent == nil {
		parent = http.DefaultTransport
	}
	return &transport{
		storage: storage,
		parent:  parent,
	}
}
