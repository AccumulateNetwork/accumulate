package e2e

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func delivered(status *protocol.TransactionStatus) bool {
	return status.Delivered
}

func TestSendTokensToBadRecipient(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitChain()

	alice := acctesting.GenerateKey("Alice")
	aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
	batch := sim.SubnetFor(aliceUrl).Database.Begin(true)
	require.NoError(t, acctesting.CreateLiteTokenAccountWithCredits(batch, tmed25519.PrivKey(alice), protocol.AcmeFaucetAmount, 1e9))
	require.NoError(t, batch.Commit())

	exch := new(protocol.SendTokens)
	exch.AddRecipient(acctesting.MustParseUrl("foo"), big.NewInt(int64(1000)))
	env := acctesting.NewTransaction().
		WithPrincipal(aliceUrl).
		WithTimestampVar(&timestamp).
		WithSigner(aliceUrl.RootIdentity(), 1).
		WithBody(exch).
		Initiate(protocol.SignatureTypeLegacyED25519, alice).
		Build()
	sim.MustSubmitAndExecuteBlock(env)
	sim.WaitForTransactionFlow(delivered, env.Transaction[0].GetHash())

	// The balance should be unchanged
	batch = sim.SubnetFor(aliceUrl).Database.Begin(false)
	defer batch.Discard()
	var account *protocol.LiteTokenAccount
	require.NoError(t, batch.Account(aliceUrl).GetStateAs(&account))
	assert.Equal(t, int64(protocol.AcmeFaucetAmount*protocol.AcmePrecision), account.Balance.Int64())

	// The synthetic transaction should fail
	synth, err := batch.Transaction(env.Transaction[0].GetHash()).GetSyntheticTxns()
	require.NoError(t, err)
	batch = sim.SubnetFor(url.MustParse("foo")).Database.Begin(false)
	defer batch.Discard()
	status, err := batch.Transaction(synth.Hashes[0][:]).GetStatus()
	require.NoError(t, err)
	assert.Equal(t, protocol.ErrorCodeNotFound.GetEnumValue(), status.Code)
}
