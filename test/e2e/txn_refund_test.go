package e2e

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestRefundCycle(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Setup accounts
	alice := protocol.AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	sim.CreateIdentity(alice, aliceKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(page *KeyPage) { page.CreditBalance = 1e9 })
	sim.CreateAccount(&TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e12)})

	// Send tokens
	txn := sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("tokens")).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestampVar(&timestamp).
			WithBody(&SendTokens{
				To: []*TokenRecipient{{
					Url:    protocol.AccountUrl("bob", "tokens"),
					Amount: *big.NewInt(1),
				}},
			}).
			Initiate(SignatureTypeED25519, aliceKey).
			Build(),
	)[0].Transaction[0]
	_, _, synth := sim.WaitForTransaction(delivered, txn.GetHash(), 50)

	// Erase the sender ADI
	_ = sim.SubnetFor(alice).Database.Update(func(batch *database.Batch) error {
		require.NoError(t, batch.DeleteAccountState_TESTONLY(alice.JoinPath("tokens")))
		require.NoError(t, batch.DeleteAccountState_TESTONLY(alice.JoinPath("book", "1")))
		require.NoError(t, batch.DeleteAccountState_TESTONLY(alice.JoinPath("book")))
		require.NoError(t, batch.DeleteAccountState_TESTONLY(alice))
		return nil
	})

	// Wait for the deposit
	var allSynth []*url.TxID
	for _, id := range synth {
		h := id.Hash()
		_, _, synth := sim.WaitForTransaction(delivered, h[:], 50)
		allSynth = append(allSynth, synth...)
	}

	// Wait for the refunds
	synth, allSynth = allSynth, nil
	for _, id := range synth {
		h := id.Hash()
		_, status, synth := sim.WaitForTransaction(delivered, h[:], 50)
		allSynth = append(allSynth, synth...)

		// Verify the refunds failed (because the account no longer exists)
		require.NotNil(t, status.Error)
		require.Equal(t, errors.StatusNotFound, status.Error.Code)
	}

	// Verify the failed refund did not generate a refund
	require.Empty(t, allSynth)
}
