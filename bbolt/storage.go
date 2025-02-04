package bboltstorage

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"os"

	"go.etcd.io/bbolt"
)

// Implements the ghtransport.Storage interface using go.etcd.io/bbolt.
type Storage struct {
	DB     *bbolt.DB
	Bucket []byte
}

func (s *Storage) Get(ctx context.Context, u *url.URL) (body io.ReadCloser, header http.Header, err error) {
	if err := s.DB.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(s.Bucket)
		if bucket == nil {
			return bbolt.ErrBucketNotFound
		}
		bodyBytes := bucket.Get([]byte(u.String()))
		if bodyBytes == nil {
			return nil
		}
		bodyBytesSafe := make([]byte, len(bodyBytes))
		copy(bodyBytesSafe, bodyBytes)
		body = io.NopCloser(bytes.NewReader(bodyBytesSafe))
		return nil
	}); err != nil {
		return nil, nil, err
	}
	return
}

func (s *Storage) Put(ctx context.Context, u *url.URL, body []byte, header http.Header) (err error) {
	return s.DB.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(s.Bucket)
		if bucket == nil {
			return bbolt.ErrBucketNotFound
		}
		return bucket.Put([]byte(u.String()), body)
	})
}

// Open is a wrapper around bbolt.Open that returns an initialized Storage.
func Open(path string, mode os.FileMode, options *bbolt.Options, bucket []byte) (*Storage, error) {
	if bucket == nil {
		bucket = []byte("github")
	}
	db, err := bbolt.Open(path, mode, options)
	if err != nil {
		return &Storage{}, err
	}
	if err := db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucket)
		return err
	}); err != nil {
		return &Storage{}, err
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
