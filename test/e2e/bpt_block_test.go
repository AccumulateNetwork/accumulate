// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"crypto/sha256"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// A NODE LOCATES A ROOT MISMATCH ALONG THE BPT'S STORAGE, OVER THE WIRE
// (#4441; executor.md, "Sync", "Two mismatches" 1).
//
// The partition holds a few hundred accounts, so its root block has branch
// positions and the walk goes below it. An account changes after block B; the
// client compares the partition's stored blocks as of B with the current ones,
// descends only under the positions that differ, and must find the account
// among the differing leaves -- having asked for a small part of the tree, not
// all of it. Every answer is checked against the position the answer above it
// gave, by folding its positions on the client side.
func TestBptBlock_LocatesAChangedAccountOverTheWire(t *testing.T) {
	liteKey := acctesting.GenerateKey(t.Name(), "lite")
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey).RootIdentity().JoinPath(ACME)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
		simulator.BPTHistoryDepth(1024),
	)
	MakeLiteTokenAccount(t, sim.DatabaseFor(lite), liteKey[32:], AcmeUrl())
	CreditCredits(t, sim.DatabaseFor(lite), lite.RootIdentity(), 1e9)
	CreditTokens(t, sim.DatabaseFor(lite), lite, big.NewInt(1e12))
	for i := 0; i < 150; i++ {
		k := acctesting.GenerateKey(t.Name(), i)
		u := acctesting.AcmeLiteAddressStdPriv(k)
		MakeLiteTokenAccount(t, sim.DatabaseFor(u), k[32:], AcmeUrl())
	}
	sim.StepN(10)

	partition, err := sim.Router().RouteAccount(lite)
	require.NoError(t, err)
	part := PartitionUrl(partition)
	c := realClient(t, sim)
	ctx := context.Background()

	// Block B, and one state-changing block after it so its root is recorded.
	B := ledgerIndexOf(t, c, partition)
	sim.StepN(3)

	// The account changes after B.
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(lite).
			AddCredits().Spend(1).To(lite.RootIdentity()).WithOracle(InitialAcmeOracle).
			SignWith(lite.RootIdentity()).Version(1).Timestamp(1).PrivateKey(liteKey))
	sim.StepUntil(Txn(st.TxID).Succeeds())
	sim.StepN(3)

	page, err := c.QueryBptPage(ctx, part, &api.BptPageQuery{Count: 4096})
	require.NoError(t, err)
	require.True(t, page.Done)
	var changed [32]byte
	for _, e := range page.Entries {
		if e.Account != nil && e.Account.Equal(lite) {
			changed = e.KeyHash
		}
	}
	require.NotZero(t, changed, "the current page does not name %v", lite)

	asked := 0
	get := func(prefix []byte, height uint64) *api.BptBlockRecord {
		t.Helper()
		asked++
		r, err := c.QueryBptBlock(ctx, part, &api.BptBlockQuery{Prefix: prefix, ForHeight: height})
		require.NoErrorf(t, err, "prefix %x as of %d", prefix, height)
		return r
	}

	then, now := get(nil, B), get(nil, 0)
	require.Equal(t, B, then.Block)
	require.Equal(t, then.BptRoot, foldSlots(then.Slots), "the root's answer as of B does not fold to B's root")
	require.Equal(t, now.BptRoot, foldSlots(now.Slots), "the current root's answer does not fold to the current root")
	require.NotEqual(t, then.BptRoot, now.BptRoot)

	// The walk: descend under every position that differs and is a branch on
	// both sides; a position that differs and is a leaf on either side names
	// an account to pull.
	located := map[[32]byte]bool{}
	branches, wholes := 0, 0
	var walk func(prefix []byte, a, b *api.BptBlockRecord)
	walk = func(prefix []byte, a, b *api.BptBlockRecord) {
		for i := 0; i < 256; i++ {
			sa, sb := positionAt(a, byte(i)), positionAt(b, byte(i))
			if sa != nil && sb != nil && sa.Equal(sb) {
				continue
			}
			if sa != nil && sb != nil && sa.Branch && sb.Branch {
				next := append(append([]byte{}, prefix...), byte(i))
				na, nb := get(next, B), get(next, 0)
				require.Equalf(t, sa.Hash, foldSlots(na.Slots), "prefix %x as of B does not fold to the position above it", next)
				require.Equalf(t, sb.Hash, foldSlots(nb.Slots), "prefix %x now does not fold to the position above it", next)
				branches++
				walk(next, na, nb)
				continue
			}
			for _, s := range []*api.BptBlockSlot{sa, sb} {
				if s != nil && !s.Branch {
					located[s.KeyHash] = true
				}
			}
			if sa != nil && sa.Branch || sb != nil && sb.Branch {
				// A branch against a leaf or nothing: the accounts under the
				// branch are what differs, and a join pulls them whole.
				wholes++
			}
		}
	}
	walk(nil, then, now)

	require.Truef(t, located[changed], "the changed account %v was not located", lite)
	require.NotZero(t, branches, "the walk never went below the root; the partition is too small to exercise the descent")
	require.Lessf(t, len(located), len(page.Entries)/2,
		"%d of %d accounts were named; locating is not cheaper than pulling", len(located), len(page.Entries))
	t.Logf("%d accounts on the partition; %d answers asked, %d blocks descended, %d leaves and %d branches named", len(page.Entries), asked, branches, len(located), wholes)
}

// THE NEWEST BLOCK'S ROOT IS NOT SERVABLE YET, AND THE ANSWER SAYS SO.
//
// The ledger records a block's BPT root when the next state-changing block
// commits, so the newest indexed block has no recorded root. A request for it
// is NotReady -- retry -- not IncompleteChain, which a requester reads as a
// capability this peer does not have. Once the next block commits, the same
// request is answered.
func TestBptBlock_TheNewestBlockIsNotReadyYet(t *testing.T) {
	liteKey := acctesting.GenerateKey(t.Name(), "lite")
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey).RootIdentity().JoinPath(ACME)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
		simulator.BPTHistoryDepth(1024),
	)
	MakeLiteTokenAccount(t, sim.DatabaseFor(lite), liteKey[32:], AcmeUrl())
	CreditCredits(t, sim.DatabaseFor(lite), lite.RootIdentity(), 1e9)
	CreditTokens(t, sim.DatabaseFor(lite), lite, big.NewInt(1e12))
	sim.StepN(10)

	partition, err := sim.Router().RouteAccount(lite)
	require.NoError(t, err)
	part := PartitionUrl(partition)
	c := realClient(t, sim)
	ctx := context.Background()

	newest := ledgerIndexOf(t, c, partition)
	r, err := c.QueryBptBlock(ctx, part, &api.BptBlockQuery{ForHeight: newest})
	require.Error(t, err, "block %d's root is not recorded yet and was answered", newest)
	require.Nil(t, r)
	require.Equalf(t, errors.NotReady, errors.Code(err), "the newest block must be NotReady, not %v: %v", errors.Code(err), err)

	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(lite).
			AddCredits().Spend(1).To(lite.RootIdentity()).WithOracle(InitialAcmeOracle).
			SignWith(lite.RootIdentity()).Version(1).Timestamp(1).PrivateKey(liteKey))
	sim.StepUntil(Txn(st.TxID).Succeeds())
	sim.StepN(3)

	r, err = c.QueryBptBlock(ctx, part, &api.BptBlockQuery{ForHeight: newest})
	require.NoError(t, err, "block %d is recorded now and was still refused", newest)
	require.Equal(t, r.BptRoot, foldSlots(r.Slots))

	// A node that retains nothing is told so, not asked to retry.
	sim2 := NewSim(t,
		simulator.SimpleNetwork(t.Name()+"2", 1, 1),
		simulator.Genesis(GenesisTime),
	)
	sim2.StepN(10)
	c2 := realClient(t, sim2)
	_, err = c2.QueryBptBlock(ctx, part, &api.BptBlockQuery{ForHeight: ledgerIndexOf(t, c2, partition)})
	require.Error(t, err)
	require.Equalf(t, errors.IncompleteChain, errors.Code(err), "%v", err)
}

func positionAt(r *api.BptBlockRecord, index byte) *api.BptBlockSlot {
	for _, s := range r.Slots {
		if s.Index == uint64(index) {
			return s
		}
	}
	return nil
}

// foldSlots folds a stored block's positions by the BPT's rule, on the client
// side: a set bit goes left, two non-empty sides hash left then right, one
// passes its hash up.
func foldSlots(slots []*api.BptBlockSlot) [32]byte {
	var h [256]*[32]byte
	for _, s := range slots {
		v := s.Hash
		h[s.Index] = &v
	}
	for n := 256; n > 1; n /= 2 {
		for i := 0; i < n/2; i++ {
			l, r := h[2*i+1], h[2*i]
			switch {
			case l != nil && r != nil:
				v := sha256.Sum256(append(append([]byte{}, l[:]...), r[:]...))
				h[i] = &v
			case l != nil:
				h[i] = l
			default:
				h[i] = r
			}
		}
	}
	if h[0] == nil {
		return [32]byte{}
	}
	return *h[0]
}
