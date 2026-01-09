package ghtransport

import (
	"net/http"
	"net/url"
	"testing"
)

func Test_cacheable(t *testing.T) {
	tests := map[string]struct {
		Request  *http.Request
		Expected bool
	}{
		"get": {
			Request: &http.Request{
				Method: "GET",
				URL: &url.URL{
					Scheme: "https",
					Host:   "api.github.com",
					Path:   "/users/bored-engineer",
				},
			},
			Expected: true,
		},
		"head": {
			Request: &http.Request{
				Method: "HEAD",
				URL: &url.URL{
					Scheme: "https",
					Host:   "api.github.com",
					Path:   "/users/bored-engineer",
				},
			},
			Expected: true,
		},
		"post": {
			Request: &http.Request{
				Method: "POST",
				URL: &url.URL{
					Scheme: "https",
					Host:   "api.github.com",
					Path:   "/users/bored-engineer",
				},
			},
			Expected: false,
		},
		"range": {
			Request: &http.Request{
				Header: http.Header{
					"Range": []string{"bytes=0-1023"},
				},
				URL: &url.URL{
					Scheme: "https",
					Host:   "api.github.com",
					Path:   "/users/bored-engineer",
				},
			},
		},
		"rate_limit": {
			Request: &http.Request{
				URL: &url.URL{
					Scheme: "https",
					Host:   "api.github.com",
					Path:   "/rate_limit",
				},
			},
			Expected: false,
		},
		"api_v3_rate_limit": {
			Request: &http.Request{
				URL: &url.URL{
					Scheme: "https",
					Host:   "api.github.com",
					Path:   "/api/v3/rate_limit",
				},
			},
			Expected: false,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := cacheable(test.Request); got != test.Expected {
				t.Errorf("Cacheable(%v) = %v, want %v", test.Request, got, test.Expected)
			}
		})
	}
}
