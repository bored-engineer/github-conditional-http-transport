package pebblestorage

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"testing"
)

var testURL = &url.URL{
	Scheme: "https",
	Host:   "api.github.com",
	Path:   "/users/bored-engineer",
}

var testBody = []byte(`{"login":"bored-engineer"}`)

func TestStorage(t *testing.T) {
	storage, err := Open(filepath.Join(t.TempDir(), "pebble.db"), nil)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() {
		if err := storage.DB.Close(); err != nil {
			t.Fatalf("(*pebble.DB).Close failed: %v", err)
		}
	}()

	if missResp, err := storage.Get(t.Context(), &http.Request{
		Method: http.MethodGet,
		URL: &url.URL{
			Scheme: "https",
			Host:   "api.github.com",
			Path:   "/invalid",
		},
	}); err != nil {
		t.Fatalf("(*Storage).Get failed: %v", err)
	} else if missResp != nil {
		t.Fatalf("(*Storage).Get returned non-nil response for invalid URL: %v", missResp)
	}

	putResp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Etag": []string{`"deadbeef"`},
		},
		Body: io.NopCloser(bytes.NewReader(testBody)),
		Request: &http.Request{
			Method: http.MethodGet,
			URL:    testURL,
		},
	}

	if err := storage.Put(t.Context(), putResp); err != nil {
		t.Fatalf("(*Storage).Put failed: %v", err)
	}

	if putBody, err := io.ReadAll(putResp.Body); err != nil {
		t.Fatalf("(*Storage).Put corrupted (*http.Response).Body: %v", err)
	} else if string(putBody) != string(testBody) {
		t.Fatalf("(*Storage).Put corrupted (*http.Response).Body: %q, want %q", string(putBody), string(testBody))
	}

	resp, err := storage.Get(t.Context(), &http.Request{
		Method: http.MethodGet,
		URL:    testURL,
	})
	if err != nil {
		t.Fatalf("(*Storage).Get failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("(*Storage).Get returned status code %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if resp.Header.Get("Etag") != `"deadbeef"` {
		t.Fatalf("(*Storage).Get returned Etag header %q, want %q", resp.Header.Get("Etag"), `"deadbeef"`)
	}
	if resp.Body == nil {
		t.Fatalf("(*Storage).Get returned nil body")
	}

	if body, err := io.ReadAll(resp.Body); err != nil {
		t.Fatalf("(*Storage).Get returned error reading body: %v", err)
	} else if string(body) != string(testBody) {
		t.Fatalf("(*Storage).Get returned body %q, want %q", string(body), string(testBody))
	}
}
