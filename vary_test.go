package ghtransport

import (
	"net/http"
	"reflect"
	"slices"
	"testing"
)

func Test_identicalVary(t *testing.T) {
	tests := map[string]struct {
		Request  *http.Request
		Cached   *http.Response
		Expected bool
	}{
		"true": {
			Request: &http.Request{
				Header: http.Header{
					"Accept": []string{"application/json"},
				},
			},
			Cached: &http.Response{
				Header: http.Header{
					VaryPrefix + "Accept": []string{"application/json"},
					"Vary":                []string{"Accept"},
				},
			},
			Expected: true,
		},
		"false": {
			Request: &http.Request{
				Header: http.Header{
					"Accept": []string{"application/json"},
				},
			},
			Cached: &http.Response{
				Header: http.Header{
					VaryPrefix + "Accept": []string{"application/xml"},
					"Vary":                []string{"Accept"},
				},
			},
			Expected: false,
		},
		"hashed": {
			Request: &http.Request{
				Header: http.Header{
					"Accept":        []string{"application/json"},
					"Authorization": []string{"Bearer hunter2"},
				},
			},
			Cached: &http.Response{
				Header: http.Header{
					VaryPrefix + "Accept":        []string{"application/json"},
					VaryPrefix + "Authorization": []string{"9S+9MrKzuG/4jvbEkGKChfSCrxXdyylUH5S89Saj9sc="},
					"Vary":                       []string{"Authorization, Accept"},
				},
			},
			Expected: true,
		},
		"multi_value_mismatch": {
			Request: &http.Request{
				Header: http.Header{
					"Accept": []string{"application/json", "application/xml"},
				},
			},
			Cached: &http.Response{
				Header: http.Header{
					VaryPrefix + "Accept": []string{"application/json"},
					"Vary":                []string{"Accept"},
				},
			},
			Expected: false,
		},
		"multi_value_match": {
			Request: &http.Request{
				Header: http.Header{
					"Accept": []string{"application/json", "application/xml"},
				},
			},
			Cached: &http.Response{
				Header: http.Header{
					VaryPrefix + "Accept": []string{"application/json", "application/xml"},
					"Vary":                []string{"Accept"},
				},
			},
			Expected: true,
		},
		"wildcard": {
			// Per RFC 9110 12.5.5, "Vary: *" means the representation may vary on
			// unspecified request headers, so it can never be treated as identical -
			// even though the request and cached response otherwise match exactly.
			Request: &http.Request{
				Header: http.Header{},
			},
			Cached: &http.Response{
				Header: http.Header{
					"Vary": []string{"*"},
				},
			},
			Expected: false,
		},
		"wildcard_with_other_headers": {
			// "*" short-circuits identicalVary regardless of where it appears in Vary
			// or whether the other listed headers actually match.
			Request: &http.Request{
				Header: http.Header{
					"Accept": []string{"application/json"},
				},
			},
			Cached: &http.Response{
				Header: http.Header{
					VaryPrefix + "Accept": []string{"application/json"},
					"Vary":                []string{"Accept, *"},
				},
			},
			Expected: false,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := identicalVary(test.Request, test.Cached); got != test.Expected {
				t.Errorf("identicalVary(%v, %v) = %v, want %v", test.Request, test.Cached, got, test.Expected)
			}
		})
	}
}

func Test_parseVary(t *testing.T) {
	tests := map[string]struct {
		Headers  http.Header
		Expected []string
	}{
		"empty": {
			Headers:  http.Header{},
			Expected: nil,
		},
		"single": {
			Headers:  http.Header{"Vary": []string{"Accept"}},
			Expected: []string{"Accept"},
		},
		"comma": {
			Headers:  http.Header{"Vary": []string{"Accept, Authorization"}},
			Expected: []string{"Accept", "Authorization"},
		},
		"multiple": {
			Headers:  http.Header{"Vary": []string{"Accept, Authorization", "Cookie"}},
			Expected: []string{"Accept", "Authorization", "Cookie"},
		},
		"trailing_comma": {
			Headers:  http.Header{"Vary": []string{"Accept,"}},
			Expected: []string{"Accept"},
		},
		"double_comma": {
			Headers:  http.Header{"Vary": []string{"Accept,,Cookie"}},
			Expected: []string{"Accept", "Cookie"},
		},
		"wildcard": {
			Headers:  http.Header{"Vary": []string{"*"}},
			Expected: []string{"*"},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := slices.Collect(parseVary(test.Headers)); !reflect.DeepEqual(got, test.Expected) {
				t.Errorf("ParseVary(%v) = %v, want %v", test.Headers, got, test.Expected)
			}
		})
	}
}
