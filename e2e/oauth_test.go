package e2e

import (
	"encoding/base64"
	"testing"
)

func TestOAuth(t *testing.T) {
	testE2E(t,
		"Basic "+base64.StdEncoding.EncodeToString([]byte(
			testEnv(t, "GH_OAUTH_ALPHA_CLIENT_ID")+":"+testEnv(t, "GH_OAUTH_ALPHA_CLIENT_SECRET"),
		)),
		"Basic "+base64.StdEncoding.EncodeToString([]byte(
			testEnv(t, "GH_OAUTH_BETA_CLIENT_ID")+":"+testEnv(t, "GH_OAUTH_BETA_CLIENT_SECRET"),
		)),
		"Basic "+base64.StdEncoding.EncodeToString([]byte(
			testEnv(t, "GH_OAUTH_GAMMA_CLIENT_ID")+":"+testEnv(t, "GH_OAUTH_GAMMA_CLIENT_SECRET"),
		)),
	)
}
