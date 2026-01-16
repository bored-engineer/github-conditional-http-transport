package e2e

import (
	"testing"
)

func TestToken(t *testing.T) {
	testE2E(t,
		"Bearer "+testEnv(t, "GH_TOKEN_ALPHA"),
		"Bearer "+testEnv(t, "GH_TOKEN_BETA"),
		"Bearer "+testEnv(t, "GH_TOKEN_GAMMA"),
	)
}
