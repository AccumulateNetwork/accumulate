package cmd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func init() {
	testMatrix.addTest(testCase4_9)
}

//testCase4_9 ED25519 test of ed25519 default signature type
func testCase4_9(t *testing.T, tc *testCmd) {

	sig := "ed25519"

	// generate protocol signature
	r, err := tc.execute(t, "key generate ed25519")
	require.NoError(t, err)
	kr := KeyResponse{}
	require.NoError(t, json.Unmarshal([]byte(r), &kr))

	// check signature type
	require.Equal(t, sig, kr.KeyType)

	t.Log(r)
}
