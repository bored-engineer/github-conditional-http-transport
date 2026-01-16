module e2e

go 1.24.0

replace github.com/bored-engineer/github-conditional-http-transport => ../

require (
	github.com/bored-engineer/github-conditional-http-transport v0.0.0-20260115233934-4e50a7c4cdaa
	github.com/google/go-github/v81 v81.0.0
	github.com/int128/oauth2-github-app v1.2.1
)

require (
	github.com/google/go-querystring v1.2.0 // indirect
	golang.org/x/oauth2 v0.34.0 // indirect
)
