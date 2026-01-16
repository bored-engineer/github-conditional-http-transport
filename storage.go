package ghtransport

import (
	"context"
	"net/http"
)

// Storage defines the interface for a storage backend.
type Storage interface {
	// Retrieves a cached HTTP response from storage for the given (*http.Request).URL.
	// If no cached HTTP response is available, it must return (nil, nil).
	Get(context.Context, *http.Request) (*http.Response, error)
	// Stores an HTTP response in storage for the given (*http.Response).Request.URL.
	// If no error is returned, the consumed (*http.Response).Body must be replaced/restored.
	Put(context.Context, *http.Response) error
}
