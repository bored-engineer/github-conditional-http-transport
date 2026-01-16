package e2e

import (
	"context"
	"testing"

	oauth2githubapp "github.com/int128/oauth2-github-app"
)

// appToken fetches an access token for the given GitHub App.
func appToken(t *testing.T, prefix string) string {
	privateKey, err := oauth2githubapp.ParsePrivateKey([]byte(testEnv(t, prefix+"_PRIVATE_KEY")))
	if err != nil {
		t.Fatalf("oauth2githubapp.ParsePrivateKey failed: %v", err)
	}
	app := &oauth2githubapp.Config{
		PrivateKey:     privateKey,
		AppID:          testEnv(t, prefix+"_CLIENT_ID"),
		InstallationID: testEnv(t, prefix+"_INSTALLATION_ID"),
	}
	token, err := app.Token(context.Background())
	if err != nil {
		t.Fatalf("(*oauth2githubapp.Config).Token failed: %v", err)
	}
	return token.AccessToken
}

func TestApp(t *testing.T) {
	testE2E(t,
		"Bearer "+appToken(t, "GH_APP_ALPHA"),
		"Bearer "+appToken(t, "GH_APP_BETA"),
		"Bearer "+appToken(t, "GH_APP_GAMMA"),
	)
}
