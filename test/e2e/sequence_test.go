package e2e

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestOutOfSequenceSynth(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	alice := acctesting.GenerateKey("Alice")
	aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
	bob := acctesting.GenerateKey("Bob")
	bobUrl := acctesting.AcmeLiteAddressStdPriv(bob)
	sim.CreateAccount(&LiteIdentity{Url: aliceUrl.RootIdentity(), CreditBalance: 1e9})
	sim.CreateAccount(&LiteTokenAccount{Url: aliceUrl, TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e9)})

	// If any envelope contains a deposit, reverse the envelopes and the
	// transactions within each
	sim.SubnetFor(bobUrl.RootIdentity()).SubmitHook = func(envelopes []*chain.Delivery) ([]*chain.Delivery, bool) {
		for _, env := range envelopes {
			if env.Transaction.Body.Type() == TransactionTypeSyntheticDepositTokens {
				for i, n := 0, len(envelopes); i < n/2; i++ {
					j := n - i - 1
					envelopes[i], envelopes[j] = envelopes[j], envelopes[i]
				}
				break
			}
		}
		return envelopes, true
	}

	// Execute
	txns := make([]*Envelope, 5)
	for i := range txns {
		txns[i] = acctesting.NewTransaction().
			WithPrincipal(aliceUrl).
			WithTimestampVar(&timestamp).
			WithSigner(aliceUrl, 1).
			WithBody(&SendTokens{
				To: []*TokenRecipient{{
					Url:    bobUrl,
					Amount: *big.NewInt(1),
				}},
			}).
			Initiate(SignatureTypeLegacyED25519, alice).
			Build()
	}
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(txns...)...)

	// Verify
	_ = sim.SubnetFor(bobUrl).Database.View(func(batch *database.Batch) error {
		var account *LiteTokenAccount
		require.NoError(t, batch.Account(bobUrl).GetStateAs(&account))
		require.Equal(t, uint64(len(txns)), account.Balance.Uint64())
		return nil
	})
}

func TestMissingSynthTxn(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	alice := acctesting.GenerateKey("Alice")
	aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
	bob := acctesting.GenerateKey("Bob")
	bobUrl := acctesting.AcmeLiteAddressStdPriv(bob)
	sim.CreateAccount(&LiteIdentity{Url: aliceUrl.RootIdentity(), CreditBalance: 1e9})
	sim.CreateAccount(&LiteTokenAccount{Url: aliceUrl, TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e9)})

	// The first time an envelope contains a deposit, drop the first deposit
	var didDrop bool
	sim.SubnetFor(bobUrl.RootIdentity()).SubmitHook = func(envelopes []*chain.Delivery) ([]*chain.Delivery, bool) {
		for i, env := range envelopes {
			if env.Transaction.Body.Type() == TransactionTypeSyntheticDepositTokens {
				fmt.Printf("Dropping %X\n", env.Transaction.GetHash()[:4])
				didDrop = true
				return append(envelopes[:i], envelopes[i+1:]...), false
			}
		}
		return envelopes, true
	}

	// Execute
	txns := make([]*Envelope, 5)
	for i := range txns {
		txns[i] = acctesting.NewTransaction().
			WithPrincipal(aliceUrl).
			WithTimestampVar(&timestamp).
			WithSigner(aliceUrl, 1).
			WithBody(&SendTokens{
				To: []*TokenRecipient{{
					Url:    bobUrl,
					Amount: *big.NewInt(1),
				}},
			}).
			Initiate(SignatureTypeLegacyED25519, alice).
			Build()
	}
	envs := sim.MustSubmitAndExecuteBlock(txns...)
	sim.ExecuteBlocks(10)
	require.True(t, didDrop, "synthetic transactions have not been sent")
	sim.WaitForTransactions(delivered, envs...)

	// Verify
	_ = sim.SubnetFor(bobUrl).Database.View(func(batch *database.Batch) error {
		var account *LiteTokenAccount
		require.NoError(t, batch.Account(bobUrl).GetStateAs(&account))
		require.Equal(t, uint64(len(txns)), account.Balance.Uint64())
		return nil
	})
}
