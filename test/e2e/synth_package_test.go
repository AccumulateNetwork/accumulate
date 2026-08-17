// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

// TestSyntheticPackageSharesOneProof proves the #4090 emission change: a block
// that produces several synthetic messages for one destination sends them in a
// package carrying ONE collection proof, instead of one envelope and one receipt
// per message.
//
// Two things have to hold, and the second is the one worth testing. The messages
// must arrive in a package with a single proof — and they must still execute,
// which means the destination really did resolve each message's proof from the
// sibling travelling with it rather than from a proof of its own.
func TestSyntheticPackageSharesOneProof(t *testing.T) {
	alice := build.
		Identity("alice").Create("book").
		Tokens("tokens").Create("ACME").Add(1e9).Identity().
		Book("book").Page(1).Create().AddCredits(1e9).Book().Identity()
	aliceKey := alice.Book("book").Page(1).GenerateKey(SignatureTypeED25519)

	bob := build.Identity("bob").Create("book").
		Tokens("tokens").Create("ACME").Identity()

	var mu sync.Mutex
	packages := 0   // envelopes carrying a shared proof
	packaged := 0   // synthetic messages that relied on one
	soloProofs := 0 // synthetics that carried their own proof

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime).With(alice, bob),
		simulator.CaptureDispatchedMessages(func(ctx context.Context, env *messaging.Envelope) (bool, error) {
			var proofs, withProof, withoutProof int
			for _, msg := range env.Messages {
				switch m := msg.(type) {
				case *messaging.SyntheticProof:
					proofs++
				case *messaging.SyntheticMessage:
					if m.Proof != nil {
						withProof++
					} else {
						withoutProof++
					}
				}
			}
			mu.Lock()
			defer mu.Unlock()
			if proofs > 0 {
				packages++
				packaged += withoutProof
				// The whole point: one proof serves the package.
				require.Equal(t, 1, proofs, "a package must carry exactly one proof")
			}
			soloProofs += withProof
			return true, nil
		}),
	)

	// Several cross-partition transfers in flight together, so at least one block
	// produces more than one synthetic for the same destination. A single
	// transfer would take the single-message path by design and prove nothing.
	var ids []*TransactionStatus
	for i := 0; i < 8; i++ {
		st := sim.BuildAndSubmitSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(uint64(i + 1)).PrivateKey(aliceKey))
		ids = append(ids, st[0])
	}
	for _, st := range ids {
		sim.StepUntil(Txn(st.TxID).Completes())
	}

	mu.Lock()
	gotPackages, gotPackaged, gotSolo := packages, packaged, soloProofs
	mu.Unlock()

	require.NotZero(t, gotPackages, "no package was ever sent — every synthetic still carried its own proof (%d of them)", gotSolo)
	require.NotZero(t, gotPackaged, "packages were sent but carried no proofless messages, so nothing shared a proof")

	// Delivery is the real assertion: bob's balance only moves if every packaged
	// message resolved its proof from the sibling and executed.
	account, err := bob.Tokens("tokens").Load(sim.DatabaseFor)
	require.NoError(t, err)
	require.Equal(t, 8, int(account.Balance.Int64()),
		"packaged synthetics did not all execute")
}

// TestSyntheticPackageSurvivesALostPackage checks that a package proves its own
// contents: losing one must not leave the others unverifiable.
//
// Note what this does NOT claim. Synthetic delivery is sequence-ordered, so a
// lost earlier message holds later ones PENDING no matter how they are proven —
// that is the sequencing rule and has nothing to do with #4090. What would be a
// #4090 failure is a surviving package being REJECTED because the proof it needed
// went missing with the package that carried it. Under a scheme where one package
// holds the proof and later ones refer back to it, that is exactly what happens.
//
// So the assertion is that everything still arrives: the hole is refilled by
// recovery and every transfer completes. If a survivor had been rejected for want
// of a proof, its stream would never drain.
func TestSyntheticPackageSurvivesALostPackage(t *testing.T) {
	alice := build.
		Identity("alice").Create("book").
		Tokens("tokens").Create("ACME").Add(1e9).Identity().
		Book("book").Page(1).Create().AddCredits(1e9).Book().Identity()
	aliceKey := alice.Book("book").Page(1).GenerateKey(SignatureTypeED25519)

	bob := build.Identity("bob").Create("book").
		Tokens("tokens").Create("ACME").Identity()

	var mu sync.Mutex
	dropped := false

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime).With(alice, bob),
		simulator.CaptureDispatchedMessages(func(ctx context.Context, env *messaging.Envelope) (bool, error) {
			isPackage := false
			for _, msg := range env.Messages {
				if _, ok := msg.(*messaging.SyntheticProof); ok {
					isPackage = true
					break
				}
			}
			if !isPackage {
				return true, nil
			}
			mu.Lock()
			defer mu.Unlock()
			if !dropped {
				dropped = true
				return false, nil // lose the first package entirely
			}
			return true, nil
		}),
	)

	// The statuses are not kept: this test asserts on the recipient's balance
	// after recovery, not on each transaction, because the dropped package is
	// meant to delay delivery rather than prevent it.
	for i := 0; i < 8; i++ {
		sim.BuildAndSubmitSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(uint64(i + 1)).PrivateKey(aliceKey))
	}

	// A dropped package leaves a hole its stream cannot see until recovery
	// notices, and the reconcile deliberately waits out a grace period before
	// believing a gap. Step well past it.
	sim.StepN(250)

	mu.Lock()
	wasDropped := dropped
	mu.Unlock()
	require.True(t, wasDropped, "no package was dropped, so nothing was tested")

	account, err := bob.Tokens("tokens").Load(sim.DatabaseFor)
	require.NoError(t, err)
	require.Equal(t, 8, int(account.Balance.Int64()),
		"the stream never drained after a package was lost — a survivor could not be verified without it")
}
