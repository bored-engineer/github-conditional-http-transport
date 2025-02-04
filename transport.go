package ghtransport

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"

	"github.com/bored-engineer/github-conditional-http-transport/internal/bufferpool"
)

type transport struct {
	storage Storage
	parent  http.RoundTripper
}

// RoundTrip implements the http.RoundTripper interface.
func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	// If the request isn't cachable (ex: POST), pass it directly to the parent transport
	if !((req.Method == "GET" || req.Method == "HEAD") && req.Header.Get("Range") == "") {
		return t.parent.RoundTrip(req)
	}

	// Check if we have a cached response available in storage
	var cachedBody *bytes.Buffer
	if body, _, err := t.storage.Get(req.Context(), req.URL); err != nil {
		return nil, fmt.Errorf("(Storage).Get failed: %w", err)
	} else if body != nil {
		defer body.Close()
		cachedBody = bufferpool.Get()
		// Calculate the _expected_ ETag from the _input_ headers but the cached body
		h := Hash(req.Header)
		if _, err := io.Copy(io.MultiWriter(cachedBody, h), body); err != nil {
			bufferpool.Put(cachedBody)
			return nil, fmt.Errorf("io.Copy failed: %w", err)
		}
		if err := body.Close(); err != nil {
			bufferpool.Put(cachedBody)
			return nil, fmt.Errorf("(io.ReadCloser).Close failed: %w", err)
		}
		// Add the If-None-Match header to the request with that calculated ETag
		req.Header.Set("If-None-Match", `"`+hex.EncodeToString(h.Sum(nil))+`"`)
	}

	// Perform the upstream request
	resp, err := t.parent.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	// If the upstream response is 304 Not Modified, we can use the cached response
	if cachedBody != nil && resp.StatusCode == http.StatusNotModified {
		// Read the response body to completion to ensure the connection can be re-used
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			return nil, fmt.Errorf("(*http.Response).Body.Read failed: %w", err)
		}
		if err := resp.Body.Close(); err != nil {
			return nil, fmt.Errorf("(*http.Response).Body.Close failed: %w", err)
		}

		// Replace the 304 with a 200 OK
		resp.Status = "200 OK"
		resp.StatusCode = http.StatusOK

		// Replace the response body with the cached body
		resp.ContentLength = int64(cachedBody.Len())
		resp.Body = &bufferpool.ReadCloser{Buffer: cachedBody}
		return resp, nil
	}

	// If the upstream response was 200 OK AND had an ETag, we can cache it
	if resp.StatusCode == http.StatusOK && resp.Header.Get("Etag") != "" {
		// Buffer the response body so we can store it AND return it to the caller
		cachedBody = bufferpool.Get()

		// Read the full response body into memory
		if _, err := io.Copy(cachedBody, resp.Body); err != nil {
			bufferpool.Put(cachedBody)
			return nil, fmt.Errorf("(*http.Response).Body.Read failed: %w", err)
		}
		if err := resp.Body.Close(); err != nil {
			bufferpool.Put(cachedBody)
			return nil, fmt.Errorf("(*http.Response).Body.Close failed: %w", err)
		}

		// Store the response body and headers in storage
		if err := t.storage.Put(req.Context(), req.URL, cachedBody.Bytes(), resp.Header); err != nil {
			bufferpool.Put(cachedBody)
			return nil, fmt.Errorf("(Storage).Put failed: %w", err)
		}

		// Return the original response with our new body
		resp.ContentLength = int64(cachedBody.Len())
		resp.Body = &bufferpool.ReadCloser{Buffer: cachedBody}
		return resp, nil
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
