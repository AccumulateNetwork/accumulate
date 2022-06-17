package chain_test

import (
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"math/big"
	"testing"
)

func BenchmarkPerformance(b *testing.B) {

	// Initialize
	sim := simulator.New(b, 3)
	sim.InitFromGenesis()

	alice := acctesting.GenerateKey("Alice")
	bob := acctesting.GenerateKey("Alice")
	charlie := acctesting.GenerateKey("Alice")

	// Create the keys and URLs
	aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
	bobUrl := acctesting.AcmeLiteAddressStdPriv(bob)
	charlieUrl := acctesting.AcmeLiteAddressStdPriv(charlie)

	batch := sim.SubnetFor(aliceUrl).Database.Begin(true)
	require.NoError(b, acctesting.CreateLiteTokenAccountWithCredits(batch, ed25519.PrivKey(alice), protocol.AcmeFaucetAmount, 1e9))
	require.NoError(b, batch.Commit())

	// Create the transaction
	delivery := acctesting.NewTransaction().
		WithPrincipal(aliceUrl).
		WithSigner(aliceUrl, 1).
		WithCurrentTimestamp().
		WithBody(&protocol.SendTokens{
			To: []*protocol.TokenRecipient{
				{Url: bobUrl, Amount: *big.NewInt(1000)},
				{Url: charlieUrl, Amount: *big.NewInt(2000)},
			},
		}).Initiate(protocol.SignatureTypeED25519, alice).BuildDelivery()

	for i := 0; i < b.N; i++ {
		batch := sim.SubnetFor(aliceUrl).Database.Begin(true)
		defer batch.Discard()
		_, err := sim.SubnetFor(aliceUrl).Executor.ProcessSignature(batch, delivery, delivery.Signatures[0])
		require.NoError(b, err)
	}
}
