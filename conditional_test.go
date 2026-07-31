package ghtransport

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/iotest"
)

func Test_addConditionalHeaders(t *testing.T) {
	tests := map[string]struct {
		Request  *http.Request
		Cached   *http.Response
		Expected string
	}{
		"nil": {
			Request:  &http.Request{Header: http.Header{}},
			Cached:   nil,
			Expected: `"4f53cda18c2baa0c0354bb5f9a3ecbe5ed12ab4d8e11ba873c2f11161202b945"`,
		},
		"vary": {
			Request: &http.Request{
				Header: http.Header{
					"Accept": []string{"application/json"},
					"Vary":   []string{"Accept"},
				},
			},
			Cached: &http.Response{
				Header: http.Header{
					"Etag":                []string{`"deadbeef"`},
					VaryPrefix + "Accept": []string{"application/json"},
					"Vary":                []string{"Accept"},
				},
				Body: io.NopCloser(bytes.NewReader([]byte("hello world"))),
			},
			Expected: `"deadbeef"`,
		},
		"calculated": {
			Request: &http.Request{
				Header: http.Header{
					"Accept":        []string{"application/json"},
					"Authorization": []string{"Bearer hunter1"},
				},
			},
			Cached: &http.Response{
				Header: http.Header{
					"Etag":                       []string{"deadbeef"},
					VaryPrefix + "Accept":        []string{"application/json"},
					VaryPrefix + "Authorization": []string{"Bearer hunter2"},
					"Vary":                       []string{"Authorization"},
				},
				Body: io.NopCloser(bytes.NewReader([]byte("hello world"))),
			},
			Expected: `"ffe6e54ebaaaff92f2feaa4bf3a5fda8ff1d49a6a4f492101039cd7c091b7523"`,
		},
		"wildcard": {
			// "Vary: *" must never reuse the cached ETag directly (identicalVary is always
			// false), and the recalculation must include every present VaryHeaders entry
			// (Accept AND Authorization here), not just the ones literally named in Vary.
			Request: &http.Request{
				Header: http.Header{
					"Accept":        []string{"application/json"},
					"Authorization": []string{"Bearer hunter1"},
				},
			},
			Cached: &http.Response{
				Header: http.Header{
					"Etag":                       []string{`"deadbeef"`},
					VaryPrefix + "Accept":        []string{"application/json"},
					VaryPrefix + "Authorization": []string{"Bearer hunter1"},
					"Vary":                       []string{"*"},
				},
				Body: io.NopCloser(bytes.NewReader([]byte("hello world"))),
			},
			Expected: `"8ba5e4f8adb9da0d9a3e374907a8afac1286f2a9eed662541449873c0bdfa27b"`,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if err := addConditionalHeaders(test.Request, test.Cached); err != nil {
				t.Fatalf("addConditionalHeaders failed: %v", err)
			}
			if inm := test.Request.Header.Get("If-None-Match"); inm != test.Expected {
				t.Errorf("addConditionalHeaders request header = %q, want %q", inm, test.Expected)
			}
			if test.Cached != nil && test.Cached.Body == nil {
				t.Fatalf("addConditionalHeaders cached body is nil")
			}
		})
	}
}

// errCloser wraps an io.Reader with a Close method that returns a fixed error.
type errCloser struct {
	io.Reader
	closeErr error
}

func (e *errCloser) Close() error { return e.closeErr }

func TestAddConditionalHeaders_BodyReadError(t *testing.T) {
	req := &http.Request{
		Header: http.Header{
			"Accept": []string{"application/json"},
		},
	}
	cached := &http.Response{
		Header: http.Header{
			// No X-Varied-Accept stored, so identicalVary is false and the body must be read.
			"Vary": []string{"Accept"},
		},
		Body: io.NopCloser(iotest.ErrReader(errors.New("read failed"))),
	}
	err := addConditionalHeaders(req, cached)
	if err == nil {
		t.Fatal("expected addConditionalHeaders to fail when the cached body fails to read")
	}
}

func TestAddConditionalHeaders_BodyCloseError(t *testing.T) {
	req := &http.Request{
		Header: http.Header{
			"Accept": []string{"application/json"},
		},
	}
	cached := &http.Response{
		Header: http.Header{
			// No X-Varied-Accept stored, so identicalVary is false and the body must be read.
			"Vary": []string{"Accept"},
		},
		Body: &errCloser{Reader: strings.NewReader("hello world"), closeErr: errors.New("close failed")},
	}
	err := addConditionalHeaders(req, cached)
	if err == nil {
		t.Fatal("expected addConditionalHeaders to fail when the cached body fails to close")
	}
}
