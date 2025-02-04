package memory

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
)

// Implements the ghtransport.Storage interface via a simple, un-bound in-memory map.
type Storage map[string][]byte

func (s Storage) Get(ctx context.Context, u *url.URL) (body io.ReadCloser, header http.Header, err error) {
	if body, ok := s[u.String()]; ok {
		return io.NopCloser(bytes.NewReader(body)), nil, nil
	}
	return nil, nil, nil
}

func (s Storage) Put(ctx context.Context, u *url.URL, body []byte, header http.Header) (err error) {
	s[u.String()] = body
	return nil
}

// NewStorage returns a new, empty Storage.
func NewStorage() Storage {
	return make(Storage)
}
