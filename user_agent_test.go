package ghtransport

import (
	"net/http"
	"reflect"
	"testing"
)

func TestReplaceUserAgent(t *testing.T) {
	tests := map[string]struct {
		Headers  http.Header
		Expected http.Header
	}{
		"empty": {
			Headers:  http.Header{},
			Expected: http.Header{},
		},
		"curl": {
			Headers: http.Header{
				"User-Agent": []string{"curl/8.1.2"},
			},
			Expected: http.Header{
				"User-Agent": []string{"cUrL/8.1.2"},
			},
		},
		"chrome": {
			Headers: http.Header{
				"User-Agent": []string{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36"},
			},
			Expected: http.Header{
				"User-Agent": []string{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 sAfArI/537.36"},
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			replaceUserAgent(test.Headers)
			if !reflect.DeepEqual(test.Headers, test.Expected) {
				t.Errorf("replaceUserAgent(%v) = %v, want %v", test.Headers, test.Headers, test.Expected)
			}
		})
	}
}
