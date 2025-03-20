package memory

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"sync"
)

// Implements the ghtransport.Storage interface via a simple, un-bound in-memory map.
type Storage struct {
	lock *sync.RWMutex
	m    map[string][]byte
}

func (s Storage) Get(ctx context.Context, u *url.URL) (body io.ReadCloser, header http.Header, err error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	if body, ok := s.m[u.String()]; ok {
		return io.NopCloser(bytes.NewReader(body)), nil, nil
	}
	return nil, nil, nil
}

func (s Storage) Put(ctx context.Context, u *url.URL, body []byte, header http.Header) (err error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.m[u.String()] = body
	return nil
}

// NewStorage returns a new, empty Storage.
func NewStorage() Storage {
	return Storage{
		lock: &sync.RWMutex{},
		m:    make(map[string][]byte),
	}
}
