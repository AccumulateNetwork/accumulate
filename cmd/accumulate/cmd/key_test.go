package cmd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	testMatrix.addTest(testCase4_9)
	testMatrix.addTest(testCase4_10)
	testMatrix.addTest(testCase4_11)
	testMatrix.addTest(testCase4_12)
	testMatrix.addTest(testCase4_13)
	testMatrix.addTest(testCase4_14)
}

//testCase4_9 ED25519 test of ed25519 default signature type
func testCase4_9(t *testing.T, tc *testCmd) {

	sig := protocol.SignatureTypeED25519

	// generate protocol signature with default signature type
	r, err := tc.execute(t, "key generate ed25519")
	require.NoError(t, err)
	kr := KeyResponse{}
	require.NoError(t, json.Unmarshal([]byte(r), &kr))

	// verify signature type
	require.Equal(t, sig, kr.KeyType)

	t.Log(r)
}

// testCase4_10 Legacyed25519ED25519 test of legacyed25519 default signature type
func testCase4_10(t *testing.T, tc *testCmd) {

	sig := protocol.SignatureTypeLegacyED25519

	// generate protocol signature with legacyed25519
	r, err := tc.execute(t, "key generate --sigtype legacyed25519 legacyed25519")
	require.NoError(t, err)
	kr := KeyResponse{}
	require.NoError(t, json.Unmarshal([]byte(r), &kr))

	// verify signature type
	require.Equal(t, sig, kr.KeyType)

	t.Log(r)
}

// testCase4_11 SignatureTypeRCD1 test of rcd1 default signature type
func testCase4_11(t *testing.T, tc *testCmd) {

	sig := protocol.SignatureTypeRCD1

	// generate protocol signature with rcd1
	r, err := tc.execute(t, "key generate --sigtype rcd1 rcd1")
	require.NoError(t, err)
	kr := KeyResponse{}
	require.NoError(t, json.Unmarshal([]byte(r), &kr))

	// verify signature type
	require.Equal(t, sig, kr.KeyType)

	t.Log(r)
}

// testCase4_12 BTCSignature test of btc default signature type
func testCase4_12(t *testing.T, tc *testCmd) {

	sig := protocol.SignatureTypeBTC

	// generate protocol signature with btc
	r, err := tc.execute(t, "key generate --sigtype btc btc")
	require.NoError(t, err)
	kr := KeyResponse{}
	require.NoError(t, json.Unmarshal([]byte(r), &kr))

	// verify signature type
	require.Equal(t, sig, kr.KeyType)

	// faucet the btc account
	_, err = tc.executeTx(t, "faucet "+kr.LiteAccount.String())
	require.NoError(t, err)

	// make sure btc account has the funds
	bal, err := testGetBalance(t, tc, kr.LiteAccount.String())
	require.NoError(t, err)
	require.Equal(t, bal, "200000000000000")

	_, err = tc.execute(t, "get "+kr.LiteAccount.String())
	require.NoError(t, err)

	_, err = tc.executeTx(t, "credits "+kr.LiteAccount.String()+" "+kr.LiteAccount.String()+" 100")
	require.NoError(t, err)

	r, err = tc.execute(t, "key generate --sigtype btc btc1")
	require.NoError(t, err)
	kr1 := KeyResponse{}
	require.NoError(t, json.Unmarshal([]byte(r), &kr1))

	// transfer from an btc based account to an btc based account
	_, err = tc.executeTx(t, "tx create "+kr.LiteAccount.String()+" "+kr1.LiteAccount.String()+" "+"100.00")
	require.NoError(t, err)

	// make sure it transferred
	bal, err = testGetBalance(t, tc, kr1.LiteAccount.String())
	require.NoError(t, err)
	require.Equal(t, bal, "10000000000")

	t.Log(r)
}

// testCase4_13 BTCLegacySignature test of btclegacy signature type
func testCase4_13(t *testing.T, tc *testCmd) {

	sig := protocol.SignatureTypeBTCLegacy

	// generate protocol signature with btclegacy
	r, err := tc.execute(t, "key generate --sigtype btclegacy btclegacy")
	require.NoError(t, err)
	kr := KeyResponse{}
	require.NoError(t, json.Unmarshal([]byte(r), &kr))

	// verify signature type
	require.Equal(t, sig, kr.KeyType)

	t.Log(r)
}

// testCase4_14 SignatureTypeETH test of eth signature type
func testCase4_14(t *testing.T, tc *testCmd) {

	sig := protocol.SignatureTypeETH

	// generate protocol signature with eth
	r, err := tc.execute(t, "key generate --sigtype eth eth")
	require.NoError(t, err)
	kr := KeyResponse{}
	require.NoError(t, json.Unmarshal([]byte(r), &kr))

	// verify signature type
	require.Equal(t, sig, kr.KeyType)

	t.Log(r)
}
