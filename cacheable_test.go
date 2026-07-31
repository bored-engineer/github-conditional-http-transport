package ghtransport

import (
	"net/http"
	"net/url"
	"testing"
)

func Test_cacheable(t *testing.T) {
	tests := map[string]struct {
		Request     *http.Request
		Expected    bool
		ExpectedFwd string
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
			Expected:    false,
			ExpectedFwd: "method",
		},
		"range": {
			Request: &http.Request{
				Method: "GET",
				Header: http.Header{
					"Range": []string{"bytes=0-1023"},
				},
				URL: &url.URL{
					Scheme: "https",
					Host:   "api.github.com",
					Path:   "/users/bored-engineer",
				},
			},
			Expected:    false,
			ExpectedFwd: "bypass",
		},
		"rate_limit": {
			Request: &http.Request{
				Method: "GET",
				URL: &url.URL{
					Scheme: "https",
					Host:   "api.github.com",
					Path:   "/rate_limit",
				},
			},
			Expected:    false,
			ExpectedFwd: "bypass",
		},
		"api_v3_rate_limit": {
			Request: &http.Request{
				Method: "GET",
				URL: &url.URL{
					Scheme: "https",
					Host:   "api.github.com",
					Path:   "/api/v3/rate_limit",
				},
			},
			Expected:    false,
			ExpectedFwd: "bypass",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, fwd := cacheable(test.Request)
			if got != test.Expected {
				t.Errorf("Cacheable(%v) = %v, want %v", test.Request, got, test.Expected)
			}
			if fwd != test.ExpectedFwd {
				t.Errorf("Cacheable(%v) fwd = %q, want %q", test.Request, fwd, test.ExpectedFwd)
			}
		})
	}
}
