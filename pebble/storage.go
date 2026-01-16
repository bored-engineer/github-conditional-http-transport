package pebblestorage

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"

	"github.com/cockroachdb/pebble/v2"
)

// Implements the ghtransport.Storage interface using github.com/cockroachdb/pebble.
type Storage struct {
	DB           *pebble.DB
	WriteOptions *pebble.WriteOptions
}

func (s *Storage) Get(ctx context.Context, req *http.Request) (_ *http.Response, rerr error) {
	key := []byte(req.URL.String())
	value, closer, err := s.DB.Get(key)
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("(*pebble.DB).Get failed: %w", err)
	}
	defer func() {
		if err := closer.Close(); err != nil && rerr == nil {
			rerr = fmt.Errorf("(*io.ReadCloser).Close failed: %w", err)
		}
	}()

	// Make a copy of the value since it's only valid until closer.Close() is called
	bodyBytes := make([]byte, len(value))
	copy(bodyBytes, value)

	resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(bodyBytes)), nil)
	if err != nil {
		return nil, fmt.Errorf("http.ReadResponse failed: %w", err)
	}
	return resp, nil
}

func (s *Storage) Put(ctx context.Context, resp *http.Response) error {
	b, err := httputil.DumpResponse(resp, true)
	if err != nil {
		return fmt.Errorf("httputil.DumpResponse failed: %w", err)
	}
	key := []byte(resp.Request.URL.String())
	if err := s.DB.Set(key, b, s.WriteOptions); err != nil {
		return fmt.Errorf("(*pebble.DB).Set failed: %w", err)
	}
	return nil
}

// Open is a wrapper around pebble.Open that returns an initialized Storage.
func Open(path string, opts *pebble.Options) (*Storage, error) {
	db, err := pebble.Open(path, opts)
	if err != nil {
		return nil, fmt.Errorf("pebble.Open failed: %w", err)
	}
	return &Storage{DB: db, WriteOptions: pebble.Sync}, nil
}

// MustOpen is a wrapper around Open that panics if an error occurs.
func MustOpen(path string, opts *pebble.Options) *Storage {
	s, err := Open(path, opts)
	if err != nil {
		panic(err)
	}
	return s
}
