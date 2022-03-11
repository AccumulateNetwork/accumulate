package protocol

import (
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFactoidAddress(t *testing.T) {
	faAddress := "FA2ybgFNYQiZFgTjkwQwp74uGsEUHJc6hGEh4YA3ai7FcssemapP"
	rcdHash, err := GetRCDFromFactoidAddress(faAddress)
	require.NoError(t, err)
	u, err := LiteTokenAddress(rcdHash, ACME)
	require.NoError(t, err)
	t.Logf("FACTOID LITE ACCOUNT ADDRESS FROM FACTOID ADDRESS: %s", u.String())

	u2, err := GetLiteAccountFromFactoidAddress(faAddress)
	require.Equal(t, u.String(), u2.String())

}

func TestRCD(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(nil)
	rcdHash := GetRCDHashFromPublicKey(pub, 0x01)

	u, err := LiteTokenAddress(rcdHash, ACME)
	require.NoError(t, err)
	t.Logf("FACTOID LITE ACCOUNT ADDRESS FROM PUBLIC KEY: %s", u.String())
}
