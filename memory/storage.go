package memory

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
)

// cachedResponse wraps a *http.Response allowing the body bytes to be stored in memory.
type cachedResponse struct {
	Response *http.Response
	Body     []byte
}

// Implements the ghtransport.Storage interface via a simple, un-bound in-memory map.
type Storage struct {
	lock *sync.RWMutex
	m    map[string]cachedResponse
}

func (s Storage) Get(ctx context.Context, u *url.URL) (*http.Response, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	body, ok := s.m[u.String()]
	if !ok {
		return nil, nil
	}
	resp := *body.Response
	resp.Body = io.NopCloser(bytes.NewReader(body.Body))
	return &resp, nil
}

func (s Storage) Put(ctx context.Context, u *url.URL, resp *http.Response) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("(*http.Response).Body.Read failed: %w", err)
	}
	if err := resp.Body.Close(); err != nil {
		return fmt.Errorf("(*http.Response).Body.Close failed: %w", err)
	}
	resp.Body = nil
	s.m[u.String()] = cachedResponse{
		Response: resp,
		Body:     body,
	}
	return nil
}

// NewStorage returns a new, empty Storage.
func NewStorage() Storage {
	return Storage{
		lock: &sync.RWMutex{},
		m:    make(map[string]cachedResponse),
	}
}
