package bboltstorage

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
	dbPath := filepath.Join(t.TempDir(), "bbolt.db")

	storage, err := Open(dbPath, 0644, nil, nil)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	// Ensure that a request for a key not in the cache return (nil, nil)
	if missResp, err := storage.Get(t.Context(), &http.Request{
		Method: http.MethodGet,
		URL:    testURL,
	}); err != nil {
		t.Fatalf("(*Storage).Get failed: %v", err)
	} else if missResp != nil {
		t.Fatalf("(*Storage).Get returned non-nil response for invalid URL: %v", missResp)
	}

	// Ensure we can put a response into the cache
	putResp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Etag": []string{`"deadbeef"`},
		},
		Body:          io.NopCloser(bytes.NewReader(testBody)),
		ContentLength: int64(len(testBody)),
		Request: &http.Request{
			Method: http.MethodGet,
			URL:    testURL,
		},
	}
	if err := storage.Put(t.Context(), putResp); err != nil {
		t.Fatalf("(*Storage).Put failed: %v", err)
	}

	// Make sure the original body was not corrupted
	if putResp.ContentLength != int64(len(testBody)) {
		t.Fatalf("(*Storage).Put corrupted ContentLength %d, want %d", putResp.ContentLength, len(testBody))
	}
	if putBody, err := io.ReadAll(putResp.Body); err != nil {
		t.Fatalf("(*Storage).Put corrupted (*http.Response).Body.Read: %v", err)
	} else if string(putBody) != string(testBody) {
		t.Fatalf("(*Storage).Put corrupted (*http.Response).Body: %q, want %q", string(putBody), string(testBody))
	}
	if err := putResp.Body.Close(); err != nil {
		t.Fatalf("(*Storage).Put corrupted (*http.Response).Body.Close: %v", err)
	}

	// Close the DB and re-open it again to ensure the data is persisted
	if err := storage.DB.Close(); err != nil {
		t.Fatalf("(*bbolt.DB).Close failed: %v", err)
	}
	storage, err = Open(dbPath, 0644, nil, nil)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	// Ensure we can retrieve the response from the cache
	getResp, err := storage.Get(t.Context(), &http.Request{
		Method: http.MethodGet,
		URL:    testURL,
	})
	if err != nil {
		t.Fatalf("(*Storage).Get failed: %v", err)
	} else if getResp == nil {
		t.Fatalf("(*Storage).Get returned nil response for valid URL: %v", getResp)
	}

	// Ensure the response is correct
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("(*Storage).Get returned status code %d, want %d", getResp.StatusCode, http.StatusOK)
	}
	if getResp.Header.Get("Etag") != `"deadbeef"` {
		t.Fatalf("(*Storage).Get returned Etag header %q, want %q", getResp.Header.Get("Etag"), `"deadbeef"`)
	}
	if getResp.Body == nil {
		t.Fatalf("(*Storage).Get returned nil body")
	}

	// Ensure the body is correct
	if getResp.ContentLength != int64(len(testBody)) {
		t.Fatalf("(*Storage).Get corrupted (*http.Response).ContentLength%d, want %d", getResp.ContentLength, len(testBody))
	}
	if body, err := io.ReadAll(getResp.Body); err != nil {
		t.Fatalf("(*Storage).Get corrupted (*http.Response).Body.Read: %v", err)
	} else if string(body) != string(testBody) {
		t.Fatalf("(*Storage).Get corrupted (*http.Response).Body: %q, want %q", string(body), string(testBody))
	}
	if err := getResp.Body.Close(); err != nil {
		t.Fatalf("(*Storage).Get corrupted (*http.Response).Body.Close: %v", err)
	}

	// Close the DB to ensure the data is persisted
	if err := storage.DB.Close(); err != nil {
		t.Fatalf("(*bbolt.DB).Close failed: %v", err)
	}
}
