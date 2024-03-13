// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	execute "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/multi"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v1/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v1/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	sortutil "gitlab.com/accumulatenetwork/accumulate/internal/util/sort"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/helpers"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
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
	txns := make([]*messaging.Envelope, 5)
	for i := range txns {
		txns[i] =
			MustBuild(t, build.Transaction().
				For(aliceUrl).
				Body(&SendTokens{
					To: []*TokenRecipient{{
						Url:    bobUrl,
						Amount: *big.NewInt(1),
					}},
				}).
				SignWith(aliceUrl).Version(1).Timestamp(&timestamp).PrivateKey(alice).Type(SignatureTypeLegacyED25519))

	}
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(txns...)...)

	// Verify
	_ = sim.PartitionFor(bobUrl).Database.View(func(batch *database.Batch) error {
		var account *LiteTokenAccount
		require.NoError(t, batch.Account(bobUrl).Main().GetAs(&account))
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
		MustBuild(t, build.Transaction().
			For(aliceUrl).
			Body(&SendTokens{
				To: []*TokenRecipient{{
					Url:    bobUrl,
					Amount: *big.NewInt(1),
				}},
			}).
			SignWith(aliceUrl).Version(1).Timestamp(&timestamp).PrivateKey(alice)),
	)
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
	simulator.QueryUrl[*api.ChainQueryResponse](sim, DnUrl(), true)

	// Submit the synthetic transaction
	sim.PartitionFor(bobUrl).Submit(false, &messaging.Envelope{
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
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	sim.CreateIdentity(alice, aliceKey[32:])
	updateAccountOld(sim, alice.JoinPath("book", "1"), func(p *KeyPage) { p.CreditBalance = 1e9 })

	faucetKey := acctesting.GenerateKey("Faucet")
	faucet := sim.CreateLiteTokenAccount(faucetKey, AcmeUrl(), 1e9, 1e15)

	// Cause a synthetic transaction
	envs := sim.MustSubmitAndExecuteBlock(
		MustBuild(t, build.Transaction().
			For(faucet).
			Body(&SendTokens{To: []*TokenRecipient{{Url: lite, Amount: *big.NewInt(1e15)}}}).
			SignWith(faucet).Version(1).Timestamp(1).PrivateKey(faucetKey)),
	)

	// Get the local anchor of the produced transaction
	var anchor, synth [32]byte
	_ = sim.PartitionFor(FaucetUrl).View(func(batch *database.Batch) error {
		produced, err := batch.Transaction(envs[0].Transaction[0].GetHash()).Produced().Get()
		require.NoError(t, err)
		require.Len(t, produced, 1)

		synth = produced[0].Hash()
		status, err := batch.Transaction(synth[:]).Status().Get()
		require.NoError(t, err)

		anchor = *(*[32]byte)(status.Proof.Anchor)
		return nil
	})

	// Drop the anchor
	var didDrop bool
	sim.PartitionFor(lite).SubmitHook = func(envelopes []*chain.Delivery) ([]*chain.Delivery, bool) {
		for i, env := range envelopes {
			body, ok := env.Transaction.Body.(*DirectoryAnchor)
			if !ok {
				continue
			}

			for _, receipt := range body.Receipts {
				if anchor == *(*[32]byte)(receipt.RootChainReceipt.Start) {
					fmt.Printf("Dropping %X with anchor %X to %X\n", env.Transaction.GetHash()[:4], anchor[:4], body.RootChainAnchor[:4])
					didDrop = true
					return append(envelopes[:i], envelopes[i+1:]...), false
				}
			}
		}
		return envelopes, true
	}
	for i := 0; i < 50 && !didDrop; i++ {
		sim.ExecuteBlock(nil)
	}
	require.True(t, didDrop, "Anchor not received within 50 blocks")

	// Do something to cause another block/anchor
	sim.MustSubmitAndExecuteBlock(
		MustBuild(t, build.Transaction().
			For(alice).
			Body(&CreateDataAccount{Url: alice.JoinPath("account")}).
			SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(1).PrivateKey(aliceKey)),
	)

	// Wait for the synthetic transaction - the BVN must be able to heal itself
	sim.WaitForTransactionFlow(delivered, synth[:])
}

func TestPoisonedAnchorTxn(t *testing.T) {
	lite := acctesting.AcmeLiteAddressStdPriv(acctesting.GenerateKey("Lite"))
	badKey := acctesting.GenerateKey("Bad")

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Poison the next anchor
	var poisoned *Transaction
	var original []*chain.Delivery
	var poison protocol.Signature
	x := sim.PartitionFor(lite)
	x.SubmitHook = func(envelopes []*chain.Delivery) ([]*chain.Delivery, bool) {
		for i, env := range envelopes {
			_, ok := env.Transaction.Body.(*DirectoryAnchor)
			if !ok {
				continue
			}

			// After poisoning, remove any other signatures
			if poisoned != nil {
				if env.Transaction.Equal(poisoned) {
					sortutil.RemoveAt(&envelopes, i)
				}
				return envelopes, true
			}

			for i, sig := range env.Signatures {
				if sig.GetTransactionHash() != env.Transaction.ID().Hash() {
					continue
				}
				sig, ok := sig.(KeySignature)
				if !ok {
					continue
				}

				// Make a copy of the original
				og := new(chain.Delivery)
				og.Transaction = env.Transaction.Copy()
				for _, sig := range env.Signatures {
					og.Signatures = append(og.Signatures, sig.CopyAsInterface().(Signature))
				}
				original = append(original, og)

				// Poison the signature
				poisoned = env.Transaction
				signer, err := new(signing.Builder).Import(sig)
				require.NoError(t, err)
				signer.SetPrivateKey(badKey)
				hash := sig.GetTransactionHash()
				poison, err = signer.Sign(hash[:])
				require.NoError(t, err)
				env.Signatures[i] = poison
				return envelopes, true
			}
		}
		return envelopes, true
	}

	// Wait for the anchor to be poisoned
	for i := 0; i < 50 && poisoned == nil; i++ {
		sim.ExecuteBlock(nil)
	}
	require.NotNil(t, poisoned, "Anchor not received within 50 blocks")

	// Execute the anchor
	sim.ExecuteBlocks(10)
	x.SubmitHook = nil

	helpers.View(t, x, func(batch *database.Batch) {
		// Verify the anchor was not processed
		status, err := batch.Transaction(poisoned.GetHash()).Status().Get()
		require.NoError(t, err)
		require.Zero(t, status.Code)

		// Verify the poison signature failed
		status, err = batch.Transaction(poison.Hash()).Status().Get()
		require.NoError(t, err)
		require.NotZero(t, status.Code)
	})

	// Resubmit the original, valid signature
	envelope := new(messaging.Envelope)
	for _, delivery := range original {
		envelope.Transaction = append(envelope.Transaction, delivery.Transaction)
		envelope.Signatures = append(envelope.Signatures, delivery.Signatures...)
	}

	results, err := (*execute.ExecutorV1)(x.Executor).Validate(envelope, false)
	require.NoError(t, err)
	for _, result := range results {
		if result.Error != nil {
			require.NoError(t, result.Error)
		}
	}
	x.Submit2(false, original)

	// Verify it is delivered
	st, _ := sim.WaitForTransactionFlow(delivered, poisoned.GetHash())
	require.Len(t, st, 1)
	require.Equal(t, errors.Delivered, st[0].Code)
}
