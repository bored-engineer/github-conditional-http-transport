package ghtransport

import (
	"context"
	"io"
	"net/http"
	"net/url"
)

// Storage defines the interface for a storage backend.
type Storage interface {
	// Retrieves a cached HTTP response body from storage for the given URL.
	// If no cached response is available, it should return (nil, nil, nil).
	Get(ctx context.Context, u *url.URL) (body io.ReadCloser, header http.Header, err error)
	// Stores an HTTP response body in storage for the given URL. It is not
	// required to retain the HTTP response headers, however it may be useful.
	Put(ctx context.Context, u *url.URL, body []byte, header http.Header) (err error)
}
