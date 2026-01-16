package redisstorage

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Key generates the Redis key from the URL.
var Key = func(req *http.Request) string {
	return strings.TrimPrefix(req.URL.String(), "https://")
}

// Storage implements the ghtransport.Storage interface backed by Redis.
type Storage struct {
	Client     *redis.Client
	Expiration time.Duration
}

func (s *Storage) Get(ctx context.Context, req *http.Request) (*http.Response, error) {
	value, err := s.Client.Get(ctx, Key(req)).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("(*redis.Client).Get failed: %w", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader([]byte(value))), nil)
	if err != nil {
		return nil, fmt.Errorf("http.ReadResponse failed: %w", err)
	}
	return resp, nil
}

func (s *Storage) Put(ctx context.Context, resp *http.Response) error {
	value, err := httputil.DumpResponse(resp, true)
	if err != nil {
		return fmt.Errorf("httputil.DumpResponse failed: %w", err)
	}
	if err := s.Client.Set(ctx, Key(resp.Request), value, s.Expiration).Err(); err != nil {
		return fmt.Errorf("(*redis.Client).Set failed: %w", err)
	}
	return nil
}

func New(client *redis.Client) *Storage {
	return &Storage{Client: client}
}
