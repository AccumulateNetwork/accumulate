//go:build testing
// +build testing

package e2e

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

func TestFactoidTestnetIsRejected(t *testing.T) {
	protocol.IsTestNet = false
	t.Cleanup(func() { protocol.IsTestNet = true })

	var timestamp uint64
	liteKey := acctesting.GenerateKey(t.Name())
	liteUrl, err := LiteTokenAddress(liteKey[32:], ACME, SignatureTypeRCD1)
	require.NoError(t, err)

	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	MakeAccount(t, sim.DatabaseFor(liteUrl),
		&LiteIdentity{Url: liteUrl.RootIdentity(), CreditBalance: 1e9, Factoid: true},
		&LiteTokenAccount{Url: liteUrl, TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e15)})

	// Execute
	st := sim.Submit(
		acctesting.NewTransaction().
			SetFactoidTestnet(). // Set the flag
			WithPrincipal(liteUrl).
			WithSigner(liteUrl, 1).
			WithTimestampVar(&timestamp).
			WithBody(&AddCredits{
				Recipient: liteUrl.RootIdentity(),
				Amount:    *big.NewInt(1 * AcmePrecision),
				Oracle:    InitialAcmeOracleValue,
			}).
			Initiate(SignatureTypeRCD1, liteKey).
			BuildDelivery())
	require.NotZero(t, st.Code)
	require.EqualError(t, st.Error, "signature 0: attempted to replay a testnet Factoid transaction")
}
