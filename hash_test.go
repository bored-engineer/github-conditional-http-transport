package ghtransport

import (
	"encoding/hex"
	"net/http"
	"testing"
)

const testBody = `{"login":"bored-engineer","id":541842,"node_id":"MDQ6VXNlcjU0MTg0Mg==","avatar_url":"https://avatars.githubusercontent.com/u/541842?v=4","gravatar_id":"","url":"https://api.github.com/users/bored-engineer","html_url":"https://github.com/bored-engineer","followers_url":"https://api.github.com/users/bored-engineer/followers","following_url":"https://api.github.com/users/bored-engineer/following{/other_user}","gists_url":"https://api.github.com/users/bored-engineer/gists{/gist_id}","starred_url":"https://api.github.com/users/bored-engineer/starred{/owner}{/repo}","subscriptions_url":"https://api.github.com/users/bored-engineer/subscriptions","organizations_url":"https://api.github.com/users/bored-engineer/orgs","repos_url":"https://api.github.com/users/bored-engineer/repos","events_url":"https://api.github.com/users/bored-engineer/events{/privacy}","received_events_url":"https://api.github.com/users/bored-engineer/received_events","type":"User","user_view_type":"public","site_admin":false,"name":"Luke Young","company":null,"blog":"https://bored.engineer/","location":"San Francisco, CA","email":null,"hireable":true,"bio":"I find bugs and exploit them. Sometimes for money, mainly for free T-Shirts...","twitter_username":null,"public_repos":136,"public_gists":51,"followers":212,"following":13,"created_at":"2010-12-30T17:15:38Z","updated_at":"2025-05-06T02:44:16Z"}`

func TestHash(t *testing.T) {
	tests := map[string]struct {
		Headers  http.Header
		Body     string
		Expected string
		Vary     []string
	}{
		"empty_all": {
			Headers:  http.Header{},
			Body:     "",
			Expected: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			Vary:     nil,
		},
		"empty_headers": {
			Headers:  http.Header{},
			Body:     testBody,
			Expected: "c5542e3ee32c0adf1128a79d80a296d03412415c924e522d9d1c75b17d7c3ef0",
			Vary:     nil,
		},
		"accept": {
			Headers: http.Header{
				"Accept": []string{"application/vnd.github.v3+json"},
			},
			Body:     testBody,
			Expected: "125f46f7d22cd8f41ea1534256ba85a45f4a0e3dcf995da9fecfe3361b93407d",
			Vary:     nil,
		},
		"vary_none": {
			Headers: http.Header{
				"Accept":        []string{"application/vnd.github.v3+json"},
				"Authorization": []string{"Bearer hunter2"},
			},
			Body:     testBody,
			Expected: "125f46f7d22cd8f41ea1534256ba85a45f4a0e3dcf995da9fecfe3361b93407d",
			Vary:     []string{"Accept"},
		},
		"vary_authorization": {
			Headers: http.Header{
				"Accept":        []string{"application/vnd.github.v3+json"},
				"Authorization": []string{"Bearer hunter2"},
			},
			Body:     testBody,
			Expected: "2c3b29a72c9c09135a89fe51c46613393b445efabdf6f02105dc1561237093a4",
			Vary:     []string{"Accept", "Authorization"},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			h := Hash(test.Headers, test.Vary)
			h.Write([]byte(test.Body))
			if got := hex.EncodeToString(h.Sum(nil)); got != test.Expected {
				t.Errorf("Hash(%v, %q) = %q, want %q", test.Headers, test.Body, got, test.Expected)
			}
		})
	}
}

func TestHashToken(t *testing.T) {
	tests := map[string]struct {
		Authorization string
		Expected      string
	}{
		"empty": {
			Authorization: "",
			Expected:      "47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU=",
		},
		"bearer": {
			Authorization: "Bearer hunter2",
			Expected:      "9S+9MrKzuG/4jvbEkGKChfSCrxXdyylUH5S89Saj9sc=",
		},
		"basic": {
			Authorization: "Basic Ym9yZWQtZW5naW5lZXI6aHVudGVyMg==",
			Expected:      "9S+9MrKzuG/4jvbEkGKChfSCrxXdyylUH5S89Saj9sc=",
		},
		"token": {
			Authorization: "token hunter2",
			Expected:      "9S+9MrKzuG/4jvbEkGKChfSCrxXdyylUH5S89Saj9sc=",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := HashToken(test.Authorization); got != test.Expected {
				t.Errorf("HashToken(%v) = %q, want %q", test.Authorization, got, test.Expected)
			}
		})
	}
}
