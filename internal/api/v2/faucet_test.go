package api_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/helpers"
)

func init() { acctesting.EnableDebugFeatures() }

func TestFaucet(t *testing.T) {
	alice := acctesting.GenerateKey(t.Name())
	aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)

	txn := &protocol.AcmeFaucet{Url: aliceUrl}
	req, payload, err := api.Package{}.ConstructFaucetTxn(txn)
	require.NoError(t, err)

	_, err = api.Package{}.ProcessExecuteRequest(req, payload)
	require.NoError(t, err)
}
