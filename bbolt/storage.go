package bboltstorage

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"

	"go.etcd.io/bbolt"
	"go.etcd.io/bbolt/errors"
)

// Implements the ghtransport.Storage interface using go.etcd.io/bbolt.
type Storage struct {
	DB     *bbolt.DB
	Bucket []byte
}

func (s *Storage) Get(ctx context.Context, u *url.URL) (*http.Response, error) {
	var bodyBytes []byte
	if err := s.DB.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(s.Bucket)
		if bucket == nil {
			return errors.ErrBucketNotFound
		}
		bodyBytesUnsafe := bucket.Get([]byte(u.String()))
		if bodyBytesUnsafe == nil {
			return nil
		}
		bodyBytes = make([]byte, len(bodyBytesUnsafe))
		copy(bodyBytes, bodyBytesUnsafe)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("(*bbolt.DB).View failed: %w", err)
	}
	if bodyBytes == nil {
		return nil, nil
	}
	resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(bodyBytes)), nil)
	if err != nil {
		return nil, fmt.Errorf("http.ReadResponse failed: %w", err)
	}
	return resp, nil
}

func (s *Storage) Put(ctx context.Context, u *url.URL, resp *http.Response) error {
	b, err := httputil.DumpResponse(resp, true)
	if err != nil {
		return fmt.Errorf("httputil.DumpResponse failed: %w", err)
	}
	if err := s.DB.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(s.Bucket)
		if bucket == nil {
			return bbolt.ErrBucketNotFound
		}
		if err := bucket.Put([]byte(u.String()), b); err != nil {
			return fmt.Errorf("(*bbolt.Bucket).Put failed: %w", err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("(*bbolt.DB).Update failed: %w", err)
	}
	return nil
}

// Open is a wrapper around bbolt.Open that returns an initialized Storage.
func Open(path string, mode os.FileMode, options *bbolt.Options, bucket []byte) (*Storage, error) {
	if bucket == nil {
		bucket = []byte("github")
	}
	db, err := bbolt.Open(path, mode, options)
	if err != nil {
		return &Storage{}, fmt.Errorf("bbolt.Open failed: %w", err)
	}
	if err := db.Update(func(tx *bbolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(bucket); err != nil {
			return fmt.Errorf("(*bbolt.Tx).CreateBucketIfNotExists failed: %w", err)
		}
		return nil
	}); err != nil {
		return &Storage{}, fmt.Errorf("(*bbolt.DB).Update failed: %w", err)
	}
	return &Storage{DB: db, Bucket: bucket}, nil
}

// MustOpen is a wrapper around Open that panics if an error occurs.
func MustOpen(path string, mode os.FileMode, options *bbolt.Options, bucket []byte) *Storage {
	s, err := Open(path, mode, options, bucket)
	if err != nil {
		panic(err)
	}
	return s
}
