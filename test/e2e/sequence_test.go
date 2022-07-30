package e2e

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
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
	sim.PartitionFor(bobUrl.RootIdentity()).SubmitHook = func(envelopes []*chain.Delivery) ([]*chain.Delivery, bool) {
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
	_ = sim.PartitionFor(bobUrl).Database.View(func(batch *database.Batch) error {
		var account *LiteTokenAccount
		require.NoError(t, batch.Account(bobUrl).GetStateAs(&account))
		require.Equal(t, uint64(len(txns)), account.Balance.Uint64())
		return nil
	})
}

func TestMissingSynthTxn(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.NewWith(t, simulator.SimulatorOptions{
		// Add more logging to debug the intermittent failure
		LogLevels: config.LogLevel{}.
			Parse(acctesting.DefaultLogLevels).
			SetModule("executor", "debug").
			SetModule("synthetic", "debug").
			String(),
	})
	sim.InitFromGenesis()

	alice := acctesting.GenerateKey("Alice")
	aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
	bob := acctesting.GenerateKey("Bob")
	bobUrl := acctesting.AcmeLiteAddressStdPriv(bob)
	sim.CreateAccount(&LiteIdentity{Url: aliceUrl.RootIdentity(), CreditBalance: 1e9})
	sim.CreateAccount(&LiteTokenAccount{Url: aliceUrl, TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e9)})

	// The first time an envelope contains a deposit, drop the first deposit
	var didDrop bool
	sim.PartitionFor(bobUrl.RootIdentity()).SubmitHook = func(envelopes []*chain.Delivery) ([]*chain.Delivery, bool) {
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
	_ = sim.PartitionFor(bobUrl).Database.View(func(batch *database.Batch) error {
		var account *LiteTokenAccount
		require.NoError(t, batch.Account(bobUrl).GetStateAs(&account))
		require.Equal(t, uint64(len(txns)), account.Balance.Uint64())
		return nil
	})
}

func TestSendSynthTxnAfterAnchor(t *testing.T) {
	// Tests AC-1860
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

	// Capture the first deposit
	var deposit *chain.Delivery
	sim.PartitionFor(bobUrl.RootIdentity()).SubmitHook = func(envelopes []*chain.Delivery) ([]*chain.Delivery, bool) {
		for i, env := range envelopes {
			if env.Transaction.Body.Type() == TransactionTypeSyntheticDepositTokens {
				fmt.Printf("Dropping %X\n", env.Transaction.GetHash()[:4])
				deposit = env
				return append(envelopes[:i], envelopes[i+1:]...), false
			}
		}
		return envelopes, true
	}

	// Execute
	envs := sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(aliceUrl).
			WithTimestampVar(&timestamp).
			WithSigner(aliceUrl, 1).
			WithBody(&SendTokens{
				To: []*TokenRecipient{{
					Url:    bobUrl,
					Amount: *big.NewInt(1),
				}},
			}).
			Initiate(SignatureTypeED25519, alice).
			Build())
	sim.WaitForTransaction(delivered, envs[0].Transaction[0].GetHash(), 50)

	// Wait for the synthetic transaction to be sent and the block to be
	// anchored
	sim.ExecuteBlocks(10)
	require.NotNil(t, deposit, "synthetic transactions have not been sent")

	// Verify the block has been anchored
	var receipt *ReceiptSignature
	for _, sig := range deposit.Signatures {
		if sig, ok := sig.(*ReceiptSignature); ok {
			receipt = sig
		}
	}
	require.NotNil(t, receipt)
	req := new(query.RequestByUrl)
	req.Url = DnUrl().JoinPath(AnchorPool).WithFragment(fmt.Sprintf("anchor/%x", receipt.Proof.Anchor))
	simulator.QueryUrl[*api.ChainQueryResponse](sim, DnUrl(), true)

	// Submit the synthetic transaction
	sim.PartitionFor(bobUrl).Submit(false, &Envelope{
		Transaction: []*Transaction{deposit.Transaction},
		Signatures:  deposit.Signatures,
	})
	sim.WaitForTransactionFlow(delivered, deposit.Transaction.GetHash())
}

func TestMissingAnchorTxn(t *testing.T) {
	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()
	liteKey := acctesting.GenerateKey("Lite")
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey)

	// Drop the anchor for the synthetic transaction
	var anchor *[32]byte
	var didDrop bool
	sim.PartitionFor(lite).SubmitHook = func(envelopes []*chain.Delivery) ([]*chain.Delivery, bool) {
		for i, env := range envelopes {
			if anchor == nil && env.Transaction.Body.Type() == TransactionTypeSyntheticDepositTokens {
				for _, sig := range env.Signatures {
					if sig, ok := sig.(*ReceiptSignature); ok {
						fmt.Printf("Received %X, want anchor %X\n", env.Transaction.GetHash()[:4], sig.Proof.Anchor[:4])
						anchor = (*[32]byte)(sig.Proof.Anchor)
						return envelopes, true
					}
				}
			}
			if anchor != nil && env.Transaction.Body.Type() == TransactionTypeDirectoryAnchor {
				body := env.Transaction.Body.(*DirectoryAnchor)
				for _, receipt := range body.Receipts {
					if *anchor == *(*[32]byte)(receipt.Start) {
						fmt.Printf("Dropping %X with anchor %X to %X\n", env.Transaction.GetHash()[:4], anchor[:4], body.RootChainAnchor[:4])
						didDrop = true
						return append(envelopes[:i], envelopes[i+1:]...), false
					}
				}
			}
		}
		return envelopes, true
	}

	// Cause a synthetic transaction
	envs := sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(FaucetUrl).
			WithBody(&AcmeFaucet{Url: lite}).
			Faucet())
	_, _, synth := sim.WaitForTransaction(delivered, envs[0].Transaction[0].GetHash(), 50)
	require.Len(t, synth, 1)

	// Wait for the anchor
	for i := 0; i < 50 && !didDrop; i++ {
		sim.ExecuteBlock(nil)
	}
	require.True(t, didDrop, "Anchor not received within 50 blocks")

	// Do something to cause another block/anchor
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(FaucetUrl).
			WithBody(&AcmeFaucet{Url: lite}).
			Faucet())...)

	// Wait for the synthetic transaction - the BVN must be able to heal itself
	h := synth[0].Hash()
	sim.WaitForTransactionFlow(delivered, h[:])
}
