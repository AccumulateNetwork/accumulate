package e2e

import (
	"bytes"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/v1"
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
	sim.SubnetFor(bobUrl.RootIdentity()).SubmitHook = func(envelopes []*Envelope) []*Envelope {
		var deposit bool
		for _, env := range envelopes {
			for _, txn := range env.Transaction {
				if txn.Body.Type() == TransactionTypeSyntheticDepositTokens {
					deposit = true
				}
			}
		}
		if !deposit {
			return envelopes
		}

		for i, n := 0, len(envelopes); i < n/2; i++ {
			j := n - i - 1
			envelopes[i], envelopes[j] = envelopes[j], envelopes[i]
		}

		for _, env := range envelopes {
			for i, n := 0, len(env.Transaction); i < n/2; i++ {
				j := n - i - 1
				env.Transaction[i], env.Transaction[j] = env.Transaction[j], env.Transaction[i]
			}
		}

		return envelopes
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
	sim.SubnetFor(bobUrl.RootIdentity()).SubmitHook = func(envelopes []*Envelope) []*Envelope {
		for _, env := range envelopes {
			var deposit int = -1
			for i, txn := range env.Transaction {
				if txn.Body.Type() == TransactionTypeSyntheticDepositTokens {
					deposit = i
					break
				}
			}

			if deposit < 0 {
				continue
			}

			if didDrop {
				return envelopes
			} else {
				didDrop = true
				fmt.Printf("Dropping %X\n", env.Transaction[deposit].GetHash()[:4])
			}

			txn := env.Transaction[deposit]
			env.Transaction = append(env.Transaction[:deposit], env.Transaction[deposit+1:]...)
			for i := 0; i < len(env.Signatures); i++ {
				sig := env.Signatures[i]
				txnHash := sig.GetTransactionHash()
				if bytes.Equal(txnHash[:], txn.GetHash()) {
					env.Signatures = append(env.Signatures[:i], env.Signatures[i+1:]...)
					i--
				}
			}
		}
		return envelopes
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
