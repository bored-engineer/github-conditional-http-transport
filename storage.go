package ghtransport

import (
	"context"
	"net/http"
	"net/url"
)

// Storage defines the interface for a storage backend.
type Storage interface {
	// Retrieves a cached HTTP response from storage for the given URL.
	// If no cached HTTP response is available, it should return (nil, nil).
	Get(context.Context, *url.URL) (*http.Response, error)
	// Stores an HTTP response in storage for the given URL.
	Put(context.Context, *url.URL, *http.Response) error
}
