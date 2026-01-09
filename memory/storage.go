package memory

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
)

// Implements the ghtransport.Storage interface via a sync.Map.
type Storage struct {
	Map sync.Map
}

func (s *Storage) Get(ctx context.Context, u *url.URL) (*http.Response, error) {
	value, ok := s.Map.Load(u.String())
	if !ok {
		return nil, nil
	}
	valueBytes, ok := value.([]byte)
	if !ok {
		return nil, fmt.Errorf("value is not a []byte")
	}
	resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(valueBytes)), nil)
	if err != nil {
		return nil, fmt.Errorf("http.ReadResponse failed: %w", err)
	}
	return resp, nil
}

func (s *Storage) Put(ctx context.Context, u *url.URL, resp *http.Response) error {
	value, err := httputil.DumpResponse(resp, true)
	if err != nil {
		return fmt.Errorf("httputil.DumpResponse failed: %w", err)
	}
	s.Map.Store(u.String(), value)
	return nil
}

// NewStorage returns a new, empty Storage.
func NewStorage() *Storage {
	return &Storage{}
}
