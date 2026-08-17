// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"math/big"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestMixedWorkloadSyntheticCoverage is the simulator counterpart of the mixed
// workload driven by test/docker/synth-heal/driver against a real network. It
// runs one transaction of each type the driver emits and proves that every
// synthetic message type the driver's verdict expects is actually produced and
// actually crosses a partition boundary.
//
// If this fails, the docker driver's coverage report is claiming to exercise a
// heal path that the protocol no longer takes — which is the failure mode that
// matters, because a driver that silently stops producing a synthetic type
// still reports every synthetic it did produce as delivered.
func TestMixedWorkloadSyntheticCoverage(t *testing.T) {
	var timestamp uint64

	// Every message the simulator dispatches from one partition to another,
	// counted by type. Anchors and the DN's own traffic land here too; we only
	// assert on the types the workload is responsible for.
	var mu sync.Mutex
	dispatched := map[string]int{}

	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.GenesisWith(GenesisTime, globals),
		simulator.SkipProposalCheck(), // FIXME should not be necessary

		simulator.CaptureDispatchedMessages(func(ctx context.Context, env *messaging.Envelope) (bool, error) {
			messages, err := env.Normalize()
			if err != nil {
				return false, err
			}
			mu.Lock()
			defer mu.Unlock()
			for _, msg := range messages {
				dispatched[dispatchedTypeName(msg)]++
			}
			return true, nil
		}),
	)

	// Place every account deliberately: the point of the test is that these
	// messages cross partitions, which only happens if the accounts do not
	// share one.
	alice := adiRoutingTo(t, sim, "alice", "BVN0")
	bob := adiRoutingTo(t, sim, "bob", "BVN1")
	charlie := adiRoutingTo(t, sim, "charlie", "BVN2")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)
	charlieKey := acctesting.GenerateKey(charlie)

	liteKey, lite := liteRoutingTo(t, sim, "BVN2")
	entry, lda := ldaRoutingTo(t, sim, "BVN2")

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e15))

	// bob/data is authorized by alice's book, which lives on another
	// partition. Signing for it therefore always crosses a boundary.
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	CreditCredits(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), 1e12)
	MakeAccount(t, sim.DatabaseFor(bob), &DataAccount{
		Url:         bob.JoinPath("data"),
		AccountAuth: AccountAuth{Authorities: []AuthorityEntry{{Url: alice.JoinPath("book")}}},
	})

	// The credit recipient has to exist before credits can be deposited on it.
	MakeLiteTokenAccount(t, sim.DatabaseFor(lite), liteKey[32:], AcmeUrl())

	// One transaction per type the docker driver emits, each aimed off-partition.
	cases := []struct {
		name  string
		build build.SignatureBuilder
	}{{
		name: "adi-transfer",
		build: build.Transaction().For(alice, "tokens").
			SendTokens(1, AcmePrecisionPower).To(lite).
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey),
	}, {
		name: "adi-create",
		build: build.Transaction().For(alice).
			CreateIdentity(charlie).WithKey(charlieKey, SignatureTypeED25519).WithKeyBook(charlie, "book").
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey),
	}, {
		name: "credit-purchase",
		build: build.Transaction().For(alice, "tokens").
			AddCredits().WithOracle(InitialAcmeOracle).Purchase(100).To(lite.RootIdentity()).
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey),
	}, {
		name: "token-burn",
		build: build.Transaction().For(alice, "tokens").
			BurnTokens(1, AcmePrecisionPower).
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey),
	}, {
		name: "data-write",
		build: build.Transaction().For(alice, "tokens").
			WriteData().Entry(entry).To(lda).
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey),
	}, {
		// The MessageForTransaction heal path: the principal is on bob's
		// partition, the only authority that can sign for it is on alice's.
		name: "cross-auth",
		build: build.Transaction().For(bob, "data").
			WriteData().DoubleHash("synth-mixed", "cross-auth").
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey),
	}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Completes is recursive over produced messages, so this covers the
			// synthetics too. cross-auth produces nothing from the transaction
			// itself — its cross-partition traffic comes from the signature.
			st := sim.BuildAndSubmitTxnSuccessfully(c.build)
			sim.StepUntil(Txn(st.TxID).Completes())
		})
	}

	// Let the last round of synthetics and their receipts settle.
	sim.StepN(20)

	mu.Lock()
	defer mu.Unlock()
	for _, want := range []string{
		TransactionTypeSyntheticDepositTokens.String(),
		TransactionTypeSyntheticCreateIdentity.String(),
		TransactionTypeSyntheticDepositCredits.String(),
		TransactionTypeSyntheticBurnTokens.String(),
		TransactionTypeSyntheticWriteData.String(),
		messaging.MessageTypeSignatureRequest.String(),
		messaging.MessageTypeCreditPayment.String(),
	} {
		require.NotZerof(t, dispatched[want], "no %s crossed a partition boundary; dispatched: %v", want, dispatched)
	}
}

// dispatchedTypeName names a dispatched message the way the driver's coverage
// report does: by transaction body type for synthetic transactions, by message
// type for everything else. Synthetics arrive wrapped in sequencing envelopes.
func dispatchedTypeName(msg messaging.Message) string {
	for {
		u, ok := msg.(interface{ Unwrap() messaging.Message })
		if !ok {
			break
		}
		msg = u.Unwrap()
	}
	if m, ok := msg.(messaging.MessageWithTransaction); ok && m.GetTransaction() != nil && m.GetTransaction().Body != nil {
		return m.GetTransaction().Body.Type().String()
	}
	return msg.Type().String()
}

// adiRoutingTo returns an ADI URL with the given prefix that routes to the
// given partition.
func adiRoutingTo(t testing.TB, sim *Sim, prefix, partition string) *url.URL {
	t.Helper()
	for i := 0; i < 1000; i++ {
		u := AccountUrl(fmt.Sprintf("%s%d", prefix, i))
		p, err := sim.Router().RouteAccount(u)
		require.NoError(t, err)
		if p == partition {
			return u
		}
	}
	t.Fatalf("no %s* ADI routes to %s", prefix, partition)
	return nil
}

// liteRoutingTo returns a key whose ACME lite token account routes to the given
// partition.
func liteRoutingTo(t testing.TB, sim *Sim, partition string) (ed25519.PrivateKey, *url.URL) {
	t.Helper()
	for i := 0; i < 1000; i++ {
		key := acctesting.GenerateKey(partition, i)
		u := acctesting.AcmeLiteAddressStdPriv(key)
		p, err := sim.Router().RouteAccount(u)
		require.NoError(t, err)
		if p == partition {
			return key, u
		}
	}
	t.Fatalf("no lite account routes to %s", partition)
	return nil, nil
}

// ldaRoutingTo returns a data entry whose lite data account routes to the given
// partition, along with that account. The entry must be the first thing written
// to the account, because its URL is derived from the entry.
func ldaRoutingTo(t testing.TB, sim *Sim, partition string) (DataEntry, *url.URL) {
	t.Helper()
	for i := 0; i < 1000; i++ {
		entry := &DoubleHashDataEntry{Data: [][]byte{
			[]byte("synth-mixed"),
			[]byte(fmt.Sprintf("%s-%d", partition, i)),
		}}
		u, err := LiteDataAddress(ComputeLiteDataAccountId(entry))
		require.NoError(t, err)
		p, err := sim.Router().RouteAccount(u)
		require.NoError(t, err)
		if p == partition {
			return entry, u
		}
	}
	t.Fatalf("no lite data account routes to %s", partition)
	return nil, nil
}
