package protocol

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestFactoidAddress(t *testing.T) {
	faAddress := "FA2ybgFNYQiZFgTjkwQwp74uGsEUHJc6hGEh4YA3ai7FcssemapP"
	rcdHash, err := GetRCDFromFactoidAddress(faAddress)
	require.NoError(t, err)

	u, err := LiteTokenAddress(rcdHash, ACME)
	require.NoError(t, err)
	t.Logf("FACTOID LITE ACCOUNT ADDRESS : %s", u.String())
}
