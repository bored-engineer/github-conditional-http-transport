package ghtransport

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/iotest"
)

// errCloseBody wraps an io.Reader with a Close method that returns a fixed error.
type errCloseBody struct {
	io.Reader
	closeErr error
}

func (e *errCloseBody) Close() error { return e.closeErr }

type mockStorage struct {
	getFunc func(context.Context, *http.Request) (*http.Response, error)
	putFunc func(context.Context, *http.Response) error
}

func (m *mockStorage) Get(ctx context.Context, req *http.Request) (*http.Response, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, req)
	}
	return nil, nil // miss
}

func (m *mockStorage) Put(ctx context.Context, resp *http.Response) error {
	if m.putFunc != nil {
		return m.putFunc(ctx, resp)
	}
	return nil
}

type mockRoundTripper struct {
	roundTripFunc func(*http.Request) (*http.Response, error)
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.roundTripFunc != nil {
		return m.roundTripFunc(req)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("default")),
	}, nil
}

func TestTransport_RoundTrip(t *testing.T) {
	tests := []struct {
		name            string
		reqMethod       string
		reqURL          string
		reqHeader       http.Header
		setupStorage    func(*testing.T) Storage
		setupParent     func(*testing.T) http.RoundTripper
		wantStatusCode  int
		wantBody        string
		wantErr         bool
		wantXCache      string // expected "X-Cache" header value
		wantCacheStatus string // expected "Cache-Status" header value
	}{
		{
			name:      "uncacheable request (POST) passes through",
			reqMethod: http.MethodPost,
			reqURL:    "https://api.github.com/repos/foo/bar",
			setupStorage: func(t *testing.T) Storage {
				return &mockStorage{
					getFunc: func(ctx context.Context, req *http.Request) (*http.Response, error) {
						t.Error("storage.Get should not be called for POST")
						return nil, nil
					},
				}
			},
			setupParent: func(t *testing.T) http.RoundTripper {
				return &mockRoundTripper{
					roundTripFunc: func(req *http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: http.StatusCreated,
							Body:       io.NopCloser(strings.NewReader("created")),
						}, nil
					},
				}
			},
			wantStatusCode:  http.StatusCreated,
			wantBody:        "created",
			wantXCache:      "MISS",
			wantCacheStatus: cacheStatusForward("method", http.StatusCreated, false),
		},
		{
			name:      "uncacheable request (POST), upstream error",
			reqMethod: http.MethodPost,
			reqURL:    "https://api.github.com/repos/foo/bar",
			setupStorage: func(t *testing.T) Storage {
				return &mockStorage{
					getFunc: func(ctx context.Context, req *http.Request) (*http.Response, error) {
						t.Error("storage.Get should not be called for POST")
						return nil, nil
					},
				}
			},
			setupParent: func(t *testing.T) http.RoundTripper {
				return &mockRoundTripper{
					roundTripFunc: func(req *http.Request) (*http.Response, error) {
						return nil, errors.New("upstream error")
					},
				}
			},
			wantErr: true,
		},
		{
			name:      "cache miss, upstream OK, stores response",
			reqMethod: http.MethodGet,
			reqURL:    "https://api.github.com/repos/foo/bar",
			setupStorage: func(t *testing.T) Storage {
				return &mockStorage{
					getFunc: func(ctx context.Context, req *http.Request) (*http.Response, error) {
						return nil, nil // miss
					},
					putFunc: func(ctx context.Context, resp *http.Response) error {
						if resp.Header.Get("Etag") != "tag1" {
							t.Errorf("expected Etag tag1 in Put, got %s", resp.Header.Get("Etag"))
						}
						// Validate body is intact or readable
						content, err := io.ReadAll(resp.Body)
						if err != nil {
							t.Errorf("failed to read response body in Put: %v", err)
						}
						if string(content) != "content" {
							t.Errorf("Put mismatch content: %s", content)
						}
						// Restore body
						resp.Body = io.NopCloser(bytes.NewReader(content))
						return nil
					},
				}
			},
			setupParent: func(t *testing.T) http.RoundTripper {
				return &mockRoundTripper{
					roundTripFunc: func(req *http.Request) (*http.Response, error) {
						resp := &http.Response{
							StatusCode:    http.StatusOK,
							Header:        http.Header{},
							Body:          io.NopCloser(strings.NewReader("content")),
							ContentLength: 7,
						}
						resp.Header.Set("Etag", "tag1")
						return resp, nil
					},
				}
			},
			wantStatusCode:  http.StatusOK,
			wantBody:        "content",
			wantXCache:      "MISS",
			wantCacheStatus: cacheStatusForward("uri-miss", http.StatusOK, true),
		},
		{
			name:      "cache miss, upstream OK with Vary, stores X-Varied-* headers",
			reqMethod: http.MethodGet,
			reqURL:    "https://api.github.com/repos/foo/bar",
			reqHeader: http.Header{
				"Accept":        []string{"application/vnd.github+json"},
				"Authorization": []string{"Bearer hunter2"},
				"Cookie":        []string{"session=abc123"},
			},
			setupStorage: func(t *testing.T) Storage {
				return &mockStorage{
					getFunc: func(ctx context.Context, req *http.Request) (*http.Response, error) {
						return nil, nil // miss
					},
					putFunc: func(ctx context.Context, resp *http.Response) error {
						if got, want := resp.Header.Get(VaryPrefix+"Accept"), "application/vnd.github+json"; got != want {
							t.Errorf("X-Varied-Accept = %q, want %q", got, want)
						}
						// Authorization is hashed before being stored so the raw token never hits storage.
						if got, want := resp.Header.Get(VaryPrefix+"Authorization"), HashToken("Bearer hunter2"); got != want {
							t.Errorf("X-Varied-Authorization = %q, want %q (hashed)", got, want)
						}
						// NOTE: unlike Authorization, Cookie is currently stored raw/unhashed even though
						// it can carry session credentials just as Authorization does. This documents the
						// current behavior rather than asserting it's correct.
						if got, want := resp.Header.Get(VaryPrefix+"Cookie"), "session=abc123"; got != want {
							t.Errorf("X-Varied-Cookie = %q, want %q (raw, current behavior)", got, want)
						}
						content, err := io.ReadAll(resp.Body)
						if err != nil {
							t.Errorf("failed to read response body in Put: %v", err)
						}
						if string(content) != "content" {
							t.Errorf("Put mismatch content: %s", content)
						}
						resp.Body = io.NopCloser(bytes.NewReader(content))
						return nil
					},
				}
			},
			setupParent: func(t *testing.T) http.RoundTripper {
				return &mockRoundTripper{
					roundTripFunc: func(req *http.Request) (*http.Response, error) {
						resp := &http.Response{
							StatusCode: http.StatusOK,
							Header: http.Header{
								"Vary": []string{"Accept, Authorization, Cookie"},
							},
							Body: io.NopCloser(strings.NewReader("content")),
						}
						resp.Header.Set("Etag", "tag1")
						return resp, nil
					},
				}
			},
			wantStatusCode:  http.StatusOK,
			wantBody:        "content",
			wantXCache:      "MISS",
			wantCacheStatus: cacheStatusForward("uri-miss", http.StatusOK, true),
		},
		{
			name:      "cache miss, speculative empty array 304",
			reqMethod: http.MethodGet,
			reqURL:    "https://api.github.com/repos/foo/bar",
			setupStorage: func(t *testing.T) Storage {
				return &mockStorage{
					getFunc: func(ctx context.Context, req *http.Request) (*http.Response, error) {
						return nil, nil // miss
					},
				}
			},
			setupParent: func(t *testing.T) http.RoundTripper {
				return &mockRoundTripper{
					roundTripFunc: func(req *http.Request) (*http.Response, error) {
						if inm := req.Header.Get("If-None-Match"); inm == "" {
							t.Errorf("expected speculative If-None-Match to be set")
						}
						return &http.Response{
							StatusCode: http.StatusNotModified,
							Header:     make(http.Header),
							Body:       io.NopCloser(strings.NewReader("")),
						}, nil
					},
				}
			},
			wantStatusCode:  http.StatusOK,
			wantBody:        "[]",
			wantXCache:      "HIT",
			wantCacheStatus: cacheStatusHitSpeculative(),
		},
		{
			name:      "storage error on Get",
			reqMethod: http.MethodGet,
			reqURL:    "https://api.github.com/repos/foo/bar",
			setupStorage: func(t *testing.T) Storage {
				return &mockStorage{
					getFunc: func(ctx context.Context, req *http.Request) (*http.Response, error) {
						return nil, errors.New("storage fail")
					},
				}
			},
			setupParent: func(t *testing.T) http.RoundTripper {
				return &mockRoundTripper{}
			},
			wantErr: true,
		},
		{
			name:      "cached body read error surfaces as RoundTrip error",
			reqMethod: http.MethodGet,
			reqURL:    "https://api.github.com/repos/foo/bar",
			reqHeader: http.Header{
				"Accept": []string{"application/vnd.github+json"},
			},
			setupStorage: func(t *testing.T) Storage {
				return &mockStorage{
					getFunc: func(ctx context.Context, req *http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: http.StatusOK,
							Header: http.Header{
								"Etag": []string{`"tag1"`},
								// No X-Varied-Accept stored, forcing identicalVary to fail and the body to be read.
								"Vary": []string{"Accept"},
							},
							Body: io.NopCloser(iotest.ErrReader(errors.New("read failed"))),
						}, nil
					},
				}
			},
			setupParent: func(t *testing.T) http.RoundTripper {
				return &mockRoundTripper{
					roundTripFunc: func(req *http.Request) (*http.Response, error) {
						t.Error("parent RoundTrip should not be called when addConditionalHeaders fails")
						return nil, errors.New("should not reach upstream")
					},
				}
			},
			wantErr: true,
		},
		{
			name:      "upstream 304 body read error",
			reqMethod: http.MethodGet,
			reqURL:    "https://api.github.com/repos/foo/bar",
			setupStorage: func(t *testing.T) Storage {
				return &mockStorage{
					getFunc: func(ctx context.Context, req *http.Request) (*http.Response, error) {
						return nil, nil // miss
					},
				}
			},
			setupParent: func(t *testing.T) http.RoundTripper {
				return &mockRoundTripper{
					roundTripFunc: func(req *http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: http.StatusNotModified,
							Header:     make(http.Header),
							Body:       io.NopCloser(iotest.ErrReader(errors.New("copy failed"))),
						}, nil
					},
				}
			},
			wantErr: true,
		},
		{
			name:      "upstream 304 body close error",
			reqMethod: http.MethodGet,
			reqURL:    "https://api.github.com/repos/foo/bar",
			setupStorage: func(t *testing.T) Storage {
				return &mockStorage{
					getFunc: func(ctx context.Context, req *http.Request) (*http.Response, error) {
						return nil, nil // miss
					},
				}
			},
			setupParent: func(t *testing.T) http.RoundTripper {
				return &mockRoundTripper{
					roundTripFunc: func(req *http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: http.StatusNotModified,
							Header:     make(http.Header),
							Body:       &errCloseBody{Reader: strings.NewReader(""), closeErr: errors.New("close failed")},
						}, nil
					},
				}
			},
			wantErr: true,
		},
		{
			name:      "HEAD cache hit returns empty body",
			reqMethod: http.MethodHead,
			reqURL:    "https://api.github.com/repos/foo/bar",
			setupStorage: func(t *testing.T) Storage {
				return &mockStorage{
					getFunc: func(ctx context.Context, req *http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: http.StatusOK,
							Status:     "200 OK",
							Header: http.Header{
								"Etag": []string{`"tag1"`},
							},
							Body:          io.NopCloser(strings.NewReader("cached content")),
							ContentLength: 14,
						}, nil
					},
				}
			},
			setupParent: func(t *testing.T) http.RoundTripper {
				return &mockRoundTripper{
					roundTripFunc: func(req *http.Request) (*http.Response, error) {
						if req.Method != http.MethodHead {
							t.Errorf("expected upstream request method HEAD, got %s", req.Method)
						}
						return &http.Response{
							StatusCode: http.StatusNotModified,
							Header:     make(http.Header),
							Body:       io.NopCloser(strings.NewReader("")),
						}, nil
					},
				}
			},
			wantStatusCode:  http.StatusOK,
			wantBody:        "",
			wantXCache:      "HIT",
			wantCacheStatus: cacheStatusHit(),
		},
		{
			name:      "upstream 304 Not Modified, cache hit",
			reqMethod: http.MethodGet,
			reqURL:    "https://api.github.com/repos/foo/bar",
			setupStorage: func(t *testing.T) Storage {
				return &mockStorage{
					getFunc: func(ctx context.Context, req *http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: http.StatusOK,
							Status:     "200 OK",
							Header: http.Header{
								"Etag": []string{`"tag1"`},
							},
							Body:          io.NopCloser(strings.NewReader("cached content")),
							ContentLength: 14,
						}, nil
					},
				}
			},
			setupParent: func(t *testing.T) http.RoundTripper {
				return &mockRoundTripper{
					roundTripFunc: func(req *http.Request) (*http.Response, error) {
						// transport logic will call addConditionalHeaders, which reads cached body if vary differs
						// Here vary is default (empty), so it should use cached Etag
						if req.Header.Get("If-None-Match") != `"tag1"` {
							t.Errorf("expected If-None-Match \"tag1\", got %q", req.Header.Get("If-None-Match"))
						}
						return &http.Response{
							StatusCode: http.StatusNotModified,
							Header:     make(http.Header),
							Body:       io.NopCloser(strings.NewReader("")),
						}, nil
					},
				}
			},
			wantStatusCode:  http.StatusOK,
			wantBody:        "cached content",
			wantXCache:      "HIT",
			wantCacheStatus: cacheStatusHit(),
		},
		{
			name:      "upstream 200 OK (modified), cache miss, stores response",
			reqMethod: http.MethodGet,
			reqURL:    "https://api.github.com/repos/foo/bar",
			setupStorage: func(t *testing.T) Storage {
				return &mockStorage{
					getFunc: func(ctx context.Context, req *http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: http.StatusOK,
							Status:     "200 OK",
							Header: http.Header{
								"Etag": []string{`"tag1"`},
							},
							Body:          io.NopCloser(strings.NewReader("old content")),
							ContentLength: 11,
						}, nil
					},
					putFunc: func(ctx context.Context, resp *http.Response) error {
						if resp.Header.Get("Etag") != "tag2" {
							t.Errorf("expected Etag tag2 in Put, got %s", resp.Header.Get("Etag"))
						}
						// consume body
						io.ReadAll(resp.Body)
						resp.Body = io.NopCloser(strings.NewReader("new content"))
						return nil
					},
				}
			},
			setupParent: func(t *testing.T) http.RoundTripper {
				return &mockRoundTripper{
					roundTripFunc: func(req *http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: http.StatusOK,
							Header:     http.Header{"Etag": []string{"tag2"}},
							Body:       io.NopCloser(strings.NewReader("new content")),
						}, nil
					},
				}
			},
			wantStatusCode:  http.StatusOK,
			wantBody:        "new content",
			wantXCache:      "MISS",
			wantCacheStatus: cacheStatusForward("stale", http.StatusOK, true),
		},
		{
			name:      "upstream error",
			reqMethod: http.MethodGet,
			reqURL:    "https://api.github.com/repos/foo/bar",
			setupStorage: func(t *testing.T) Storage {
				return &mockStorage{
					getFunc: func(ctx context.Context, req *http.Request) (*http.Response, error) {
						return nil, nil // miss
					},
				}
			},
			setupParent: func(t *testing.T) http.RoundTripper {
				return &mockRoundTripper{
					roundTripFunc: func(req *http.Request) (*http.Response, error) {
						return nil, errors.New("upstream error")
					},
				}
			},
			wantErr: true,
		},
		{
			name:      "upstream error with cached response",
			reqMethod: http.MethodGet,
			reqURL:    "https://api.github.com/repos/foo/bar",
			setupStorage: func(t *testing.T) Storage {
				return &mockStorage{
					getFunc: func(ctx context.Context, req *http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(strings.NewReader("cached response")),
						}, nil
					},
				}
			},
			setupParent: func(t *testing.T) http.RoundTripper {
				return &mockRoundTripper{
					roundTripFunc: func(req *http.Request) (*http.Response, error) {
						return nil, errors.New("upstream error")
					},
				}
			},
			wantErr: true,
		},
		{
			name:      "nil storage, speculative empty array 304",
			reqMethod: http.MethodGet,
			reqURL:    "https://api.github.com/repos/foo/bar",
			setupStorage: func(t *testing.T) Storage {
				return nil
			},
			setupParent: func(t *testing.T) http.RoundTripper {
				return &mockRoundTripper{
					roundTripFunc: func(req *http.Request) (*http.Response, error) {
						if inm := req.Header.Get("If-None-Match"); inm == "" {
							t.Errorf("expected speculative If-None-Match to be set")
						}
						return &http.Response{
							StatusCode: http.StatusNotModified,
							Header:     make(http.Header),
							Body:       io.NopCloser(strings.NewReader("")),
						}, nil
					},
				}
			},
			wantStatusCode:  http.StatusOK,
			wantBody:        "[]",
			wantXCache:      "HIT",
			wantCacheStatus: cacheStatusHitSpeculative(),
		},
		{
			name:      "nil storage, upstream 200 OK is not stored",
			reqMethod: http.MethodGet,
			reqURL:    "https://api.github.com/repos/foo/bar",
			setupStorage: func(t *testing.T) Storage {
				return nil
			},
			setupParent: func(t *testing.T) http.RoundTripper {
				return &mockRoundTripper{
					roundTripFunc: func(req *http.Request) (*http.Response, error) {
						resp := &http.Response{
							StatusCode:    http.StatusOK,
							Header:        http.Header{},
							Body:          io.NopCloser(strings.NewReader("content")),
							ContentLength: 7,
						}
						resp.Header.Set("Etag", "tag1")
						return resp, nil
					},
				}
			},
			wantStatusCode:  http.StatusOK,
			wantBody:        "content",
			wantXCache:      "MISS",
			wantCacheStatus: cacheStatusForward("uri-miss", http.StatusOK, false),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panic caught: %v", r)
				}
			}()

			storage := tt.setupStorage(t)
			parent := tt.setupParent(t)
			tr := NewTransport(storage, parent)

			req, err := http.NewRequest(tt.reqMethod, tt.reqURL, nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}
			if tt.reqHeader != nil {
				req.Header = tt.reqHeader
			}

			resp, err := tr.RoundTrip(req)
			if (err != nil) != tt.wantErr {
				t.Errorf("RoundTrip() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if resp == nil {
				t.Fatal("expected response, got nil")
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatusCode {
				t.Errorf("RoundTrip() status = %v, want %v", resp.StatusCode, tt.wantStatusCode)
			}

			body, _ := io.ReadAll(resp.Body)
			if string(body) != tt.wantBody {
				t.Errorf("RoundTrip() body = %q, want %q", string(body), tt.wantBody)
			}

			if got := resp.Header.Get("X-Cache"); got != tt.wantXCache {
				t.Errorf("RoundTrip() %s = %q, want %q", "X-Cache", got, tt.wantXCache)
			}
			if got := resp.Header.Get("Cache-Status"); got != tt.wantCacheStatus {
				t.Errorf("RoundTrip() %s = %q, want %q", "Cache-Status", got, tt.wantCacheStatus)
			}
		})
	}
}

func TestCacheName_override(t *testing.T) {
	original := CacheName
	CacheName = "my-custom-cache"
	defer func() { CacheName = original }()

	tr := NewTransport(
		&mockStorage{},
		&mockRoundTripper{
			roundTripFunc: func(req *http.Request) (*http.Response, error) {
				resp := &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{},
					Body:       io.NopCloser(strings.NewReader("content")),
				}
				resp.Header.Set("Etag", "tag1")
				return resp, nil
			},
		},
	)

	req, err := http.NewRequest(http.MethodGet, "https://api.github.com/repos/foo/bar", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}
	defer resp.Body.Close()

	want := `my-custom-cache; fwd=uri-miss; fwd-status=200; stored`
	if got := resp.Header.Get("Cache-Status"); got != want {
		t.Errorf("RoundTrip() %s = %q, want %q", "Cache-Status", got, want)
	}
}

func TestNewTransport_defaultParent(t *testing.T) {
	tr := NewTransport(&mockStorage{}, nil)
	impl, ok := tr.(*transport)
	if !ok {
		t.Fatalf("NewTransport() returned %T, want *transport", tr)
	}
	if impl.parent != http.DefaultTransport {
		t.Errorf("NewTransport() parent = %v, want http.DefaultTransport", impl.parent)
	}
}

// TestTransport_RoundTrip_PutError documents current behavior when Storage.Put fails:
// RoundTrip returns a non-nil *http.Response alongside the error. Most http.RoundTripper
// callers (including net/http.Client) discard/never close the response when err != nil,
// which can leak the underlying body/connection.
func TestTransport_RoundTrip_PutError(t *testing.T) {
	storage := &mockStorage{
		getFunc: func(ctx context.Context, req *http.Request) (*http.Response, error) {
			return nil, nil // miss
		},
		putFunc: func(ctx context.Context, resp *http.Response) error {
			return errors.New("put failed")
		},
	}
	parent := &mockRoundTripper{
		roundTripFunc: func(req *http.Request) (*http.Response, error) {
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader("content")),
			}
			resp.Header.Set("Etag", "tag1")
			return resp, nil
		},
	}
	tr := NewTransport(storage, parent)

	req, err := http.NewRequest(http.MethodGet, "https://api.github.com/repos/foo/bar", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := tr.RoundTrip(req)
	if err == nil {
		t.Fatal("expected RoundTrip to return an error when Storage.Put fails")
	}
	if resp == nil {
		t.Fatal("expected RoundTrip to return a non-nil response alongside the Put error (current behavior)")
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		t.Fatalf("failed to read response body: %v", readErr)
	}
	if string(body) != "content" {
		t.Errorf("response body = %q, want %q", body, "content")
	}
}
