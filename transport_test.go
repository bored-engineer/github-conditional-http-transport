package ghtransport

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

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
		name           string
		reqMethod      string
		reqURL         string
		reqHeader      http.Header
		setupStorage   func(*testing.T) Storage
		setupParent    func(*testing.T) http.RoundTripper
		wantStatusCode int
		wantBody       string
		wantErr        bool
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
			wantStatusCode: http.StatusCreated,
			wantBody:       "created",
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
			wantStatusCode: http.StatusOK,
			wantBody:       "content",
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
			wantStatusCode: http.StatusOK,
			wantBody:       "cached content",
		},
		{
			name:      "upstream 200 OK (modified), cache hit",
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
			wantStatusCode: http.StatusOK,
			wantBody:       "new content",
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
		})
	}
}
