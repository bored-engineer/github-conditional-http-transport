module e2e

go 1.25.5

replace github.com/bored-engineer/github-conditional-http-transport => ../

require (
	github.com/bored-engineer/github-conditional-http-transport v0.0.0-00010101000000-000000000000
	github.com/google/go-github/v81 v81.0.0
	github.com/int128/oauth2-github-app v1.1.2
)

require (
	github.com/google/go-querystring v1.1.0 // indirect
	golang.org/x/oauth2 v0.34.0 // indirect
)
