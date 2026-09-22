// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"bytes"
	"context"
	"crypto/sha256"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiv3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	dbmerkle "gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A PEER SERVES AN ACCOUNT AND A BPT PAGE AS OF A BLOCK THE DIRECTORY ANCHORED
// (#4361).
//
// executor.md, "Sync", §2: a pulled account is kept only if its receipt "ends
// at the `StateTreeAnchor` the verified anchor carries". Before this branch a
// peer could only answer with a receipt to its OWN CURRENT root, so an account
// that changes every block could never be settled — by the time the anchor for
// the block it was served at arrived, the peer had moved on — and a restart
// converged on nothing.
//
// Nothing here is built by hand. The network is the daemon's own start path,
// the client is the real JSON-RPC client over HTTP against the node's own API,
// the anchors are read the way pull/verify.go reads them (the Directory's
// anchor pool, main chain, expanded), and the querier is the one
// `(*Querier).start` registered.
//
// The account asked about is `bvn-BVN1/ledger`, on purpose. It is the hottest
// account on the partition — every block writes it — so "as of block B" and
// "as of now" are different answers, and a node that quietly served the
// current state would be caught by the anchor comparison rather than pass by
// luck.
func TestAnAccountIsServedAsOfTheBlockTheDirectoryAnchored(t *testing.T) {
	c := clientFor(t, startNetsimAndExecute(t))
	ctx := context.Background()

	anchors := anchorsForBVN1(t, c)
	require.NotEmpty(t, anchors, "the Directory has executed no BVN1 anchor, so there is nothing anchored to serve at")

	ledger := protocol.PartitionUrl("BVN1").JoinPath(protocol.Ledger)
	served := 0
	var lastServed uint64
	for _, a := range anchors {
		r, err := c.QueryAccount(ctx, ledger, &apiv3.DefaultQuery{
			IncludeReceipt: &apiv3.ReceiptOptions{ForHeight: a.block},
		})
		if err != nil {
			// A refusal is accepted only BEFORE the first block the node
			// could serve. The retained window is a range with one floor, so
			// once a block has been answered every later one must be too; a
			// refusal after that is a hole, and a hole is what an off-by-one
			// between the bpt chain and the root index chain would look like.
			// Logging it and moving on is how the alignment claim went
			// unasserted while the comment said a test made it.
			require.Zerof(t, served,
				"block %d was refused after block %d was served, so the retained window has a hole in it: %v",
				a.block, lastServed, err)
			t.Logf("block %d (before the node's first answerable block): %v", a.block, err)
			continue
		}
		require.NotNil(t, r.Receipt, "block %d was answered with no receipt", a.block)

		// THE CLAIM. The receipt ends where the verified anchor says the
		// partition's state tree stood at that block, so a node holding that
		// anchor settles the account on the round it fetched it.
		require.Equalf(t, a.stateTreeAnchor[:], r.Receipt.Anchor,
			"the receipt for block %d does not end at that block's StateTreeAnchor", a.block)

		// And it says which block it is really for -- the last state-changing
		// block at or before the one asked about, never a later one.
		require.NotZero(t, r.Receipt.ForHeight, "the receipt does not say which block it is for")
		require.LessOrEqual(t, r.Receipt.ForHeight, a.block)

		// And it checks with nothing but itself.
		require.True(t, r.Receipt.Validate(nil), "the receipt for block %d does not verify offline", a.block)

		// And where the node retained the state receipt, the starting point is
		// one the caller can recompute from the body it was just handed --
		// which is the whole point of proving an account rather than trusting
		// the server for it.
		if r.Receipt.StartsAtMainState {
			body, err := r.Account.MarshalBinary()
			require.NoError(t, err)
			h := sha256.Sum256(body)
			require.Equalf(t, h[:], r.Receipt.Start,
				"the receipt for block %d claims to start at the main state, but not at the main state it served", a.block)
		}
		served++
		lastServed = a.block
	}
	require.NotZero(t, served, "not one anchored block could be served")
	t.Logf("served %d of %d anchored blocks, last %d", served, len(anchors), lastServed)
}

// THE PAST IS NOT ANSWERED WITH THE PRESENT.
//
// The other half of the claim above, and the one a silent degradation would
// slip past: the ledger changes every block, so the receipt for an anchored
// block must differ from the receipt for now. A node that ignored ForHeight
// would return the same anchor for both and pass every assertion about
// validity while proving the wrong thing -- which is what this line did before
// this branch, because ReceiptOptions.Yes() already reported true for a
// ForHeight-only query and the account path built a current-state receipt
// anyway.
func TestAHistoricalReceiptIsNotTheCurrentOne(t *testing.T) {
	c := clientFor(t, startNetsimAndExecute(t))
	ctx := context.Background()

	anchors := anchorsForBVN1(t, c)
	require.NotEmpty(t, anchors)
	ledger := protocol.PartitionUrl("BVN1").JoinPath(protocol.Ledger)

	now, err := c.QueryAccount(ctx, ledger, &apiv3.DefaultQuery{
		IncludeReceipt: &apiv3.ReceiptOptions{ForAny: true},
	})
	require.NoError(t, err)
	require.NotNil(t, now.Receipt)
	require.Zero(t, now.Receipt.ForHeight, "a current-state receipt must not claim a height")

	// The oldest anchored block this node can still serve
	var past *apiv3.AccountRecord
	var pastBlock uint64
	for _, a := range anchors {
		r, err := c.QueryAccount(ctx, ledger, &apiv3.DefaultQuery{
			IncludeReceipt: &apiv3.ReceiptOptions{ForHeight: a.block},
		})
		if err == nil {
			past, pastBlock = r, a.block
			break
		}
	}
	require.NotNil(t, past, "no anchored block could be served")

	require.NotEqual(t, now.Receipt.Anchor, past.Receipt.Anchor,
		"the receipt for block %d ends at the same root as the current one: the past was answered with the present", pastBlock)
	require.Equal(t, uint64(0), now.Receipt.ForHeight)
	require.NotZero(t, past.Receipt.ForHeight)
}

// A BPT PAGE IS SERVED AS OF THE SAME ANCHORED BLOCK.
//
// A page taken from the peer's current tree names the accounts the peer holds
// NOW, so the difference a joining node computes from it is a difference
// against a moving target and the leaves it names are leaves no anchor covers.
// At a named block the page is the tree the anchored root commits to.
//
// AIP-58 on main does not cover this; it is the second of the two things the
// port had to add.
//
// # This asserts on a LEAF, because BptRoot is a label
//
// The first version of this test asserted `page.BptRoot == StateTreeAnchor`
// and `len(Entries) > 0`, and the reviewer's mutation M4b walked through it:
// `BptRoot` is copied from the ledger's bpt chain by the server
// (`queryBptPageAt`), not derived from the page, so a page of the CURRENT tree
// wearing block B's label passed. A page carries no proof of its own, so
// nothing downstream would have caught it either.
//
// So the assertion is on the hottest leaf on the partition — the ledger's,
// which changes every block — and it is checked two ways, neither of which the
// server can satisfy by copying a label: the leaf must differ from the same
// leaf now, and it must be the leaf the ACCOUNT query for the same block
// proves, which arrives by an independent path (a receipt the client folds).
func TestABptPageIsServedAsOfTheBlockTheDirectoryAnchored(t *testing.T) {
	c := clientFor(t, startNetsimAndExecute(t))
	ctx := context.Background()

	anchors := anchorsForBVN1(t, c)
	require.NotEmpty(t, anchors)
	part := protocol.PartitionUrl("BVN1")
	ledger := part.JoinPath(protocol.Ledger)

	// One page big enough to hold the whole tree, so the ledger's leaf is on
	// it without paging.
	const whole = 4096
	now, err := c.Query(ctx, part, &apiv3.BptPageQuery{Count: whole})
	require.NoError(t, err)
	nowPage, ok := now.(*apiv3.BptPageRecord)
	require.True(t, ok)
	require.True(t, nowPage.Done, "the current page did not exhaust the tree; the test cannot find its leaf")
	nowLeaf, ok := leafFor(nowPage, ledger)
	require.True(t, ok, "the current page does not name %v", ledger)

	served := 0
	for _, a := range anchors {
		r, err := c.Query(ctx, part, &apiv3.BptPageQuery{Count: whole, ForHeight: a.block})
		if err != nil {
			require.Zerof(t, served,
				"block %d's page was refused after an earlier block's was served: %v", a.block, err)
			t.Logf("block %d (before the node's first answerable block): %v", a.block, err)
			continue
		}
		page, ok := r.(*apiv3.BptPageRecord)
		require.True(t, ok)

		// The label. Necessary, and on its own worth nothing.
		require.Equalf(t, a.stateTreeAnchor, page.BptRoot,
			"the page for block %d is not labelled with that block's StateTreeAnchor", a.block)
		require.NotEmpty(t, page.Entries, "the page for block %d named no accounts", a.block)

		// THE CLAIM. The tree behind the label is that block's tree, said by a
		// leaf that has moved since.
		leaf, ok := leafFor(page, ledger)
		require.Truef(t, ok, "the page for block %d does not name %v", a.block, ledger)
		require.NotEqualf(t, nowLeaf.ValueHash, leaf.ValueHash,
			"the page for block %d carries %v's CURRENT leaf: the label is historical and the tree is not", a.block, ledger)

		// And the same leaf, reached the other way: the account query at that
		// block returns a receipt from the body's hash to the block's root, and
		// the account's BPT entry is one of the values on that path. A page
		// built from a different tree names a leaf that is not on it.
		acct, err := c.QueryAccount(ctx, ledger, &apiv3.DefaultQuery{
			IncludeReceipt: &apiv3.ReceiptOptions{ForHeight: a.block},
		})
		require.NoErrorf(t, err, "the page for block %d was served but the account was not", a.block)
		// Both resolve the same height, or the two answers are about
		// different blocks and comparing them says nothing.
		require.Equalf(t, a.block, acct.Receipt.ForHeight,
			"the account and the page resolved block %d differently", a.block)
		require.Truef(t, receiptPassesThrough(&acct.Receipt.Receipt, leaf.ValueHash),
			"%v's leaf on the page for block %d is not on the path the account's own receipt for that block proves",
			ledger, a.block)

		served++
	}
	require.NotZero(t, served, "not one anchored block's page could be served")
	t.Logf("served %d of %d anchored pages; %v's leaf now %x", served, len(anchors), ledger, nowLeaf.ValueHash[:8])
}

// leafFor finds an account's leaf on a page.
func leafFor(page *apiv3.BptPageRecord, u *url.URL) (*apiv3.BptLeafSummary, bool) {
	for _, e := range page.Entries {
		if e.Account != nil && e.Account.Equal(u) {
			return e, true
		}
	}
	return nil, false
}

// receiptPassesThrough folds a receipt from its start and reports whether the
// given hash is one of the values on the way. It is how a client checks that
// the chain from the body runs through the account's BPT entry rather than
// arriving at the right anchor by some other route.
func receiptPassesThrough(r *merkle.Receipt, want [32]byte) bool {
	v := r.Start
	if bytes.Equal(v, want[:]) {
		return true
	}
	for _, e := range r.Entries {
		var b []byte
		if e.Right {
			b = append(append(b, v...), e.Hash...)
		} else {
			b = append(append(b, e.Hash...), v...)
		}
		h := sha256.Sum256(b)
		v = h[:]
		if bytes.Equal(v, want[:]) {
			return true
		}
	}
	return false
}

// A NODE THAT RETAINS NOTHING REFUSES, AND DOES NOT APPROXIMATE.
//
// This is main's TestAIP58_RefusesWhatItCannotProve ported: the operationally
// important half. Answering a question about the past with the present root
// would be confidently wrong, which is worse than an error. A node configured
// with a depth of zero must decline every historical question while its
// current-state answers keep working.
func TestANodeThatRetainsNoBptHistoryRefuses(t *testing.T) {
	depth := uint64(0)
	c := clientFor(t, startNetsimAndExecuteWith(t, &depth))
	ctx := context.Background()

	ledger := protocol.PartitionUrl("BVN1").JoinPath(protocol.Ledger)
	part := protocol.PartitionUrl("BVN1")

	now, err := c.QueryAccount(ctx, ledger, &apiv3.DefaultQuery{
		IncludeReceipt: &apiv3.ReceiptOptions{ForAny: true},
	})
	require.NoError(t, err, "the current-state path must keep working")
	require.NotNil(t, now.Receipt)

	for _, h := range []uint64{1, 3, 5, 8} {
		r, err := c.QueryAccount(ctx, ledger, &apiv3.DefaultQuery{
			IncludeReceipt: &apiv3.ReceiptOptions{ForHeight: h},
		})
		require.Errorf(t, err, "block %d was answered by a node that retains nothing", h)
		require.Nil(t, r)

		p, err := c.Query(ctx, part, &apiv3.BptPageQuery{Count: 4, ForHeight: h})
		require.Errorf(t, err, "a page for block %d was served by a node that retains nothing", h)
		require.Nil(t, p)
	}
}

// A BLOCK OUTSIDE THE RETAINED WINDOW IS REFUSED, AND THE REFUSAL NAMES THE
// WINDOW.
//
// The window is configuration, and a client planning around it needs to be
// told where it ends rather than left to probe. The refusal is
// IncompleteChain (414), a capability limit -- NOT NotFound, which this path
// reserves for proven absence (the account had no record at that height). The
// issue's done-when says "NotFound that says so"; the four-status split is
// main's and it is worth keeping, so this is a deliberate difference and it is
// recorded on #4361.
func TestABlockBelowTheRetentionHorizonIsRefusedByName(t *testing.T) {
	depth := uint64(4)
	c := clientFor(t, startNetsimAndExecuteWith(t, &depth))
	ctx := context.Background()

	ledger := protocol.PartitionUrl("BVN1").JoinPath(protocol.Ledger)

	now, err := c.QueryAccount(ctx, ledger, &apiv3.DefaultQuery{
		IncludeReceipt: &apiv3.ReceiptOptions{ForAny: true},
	})
	require.NoError(t, err, "the current-state path must keep working")
	require.NotNil(t, now.Receipt)

	// Block 1 is indexed -- the node has executed dozens of blocks since --
	// and it is far below a four-block window.
	_, err = c.QueryAccount(ctx, ledger, &apiv3.DefaultQuery{
		IncludeReceipt: &apiv3.ReceiptOptions{ForHeight: 1},
	})
	require.Error(t, err, "a block far below the retention horizon was answered")
	require.Equalf(t, errors.IncompleteChain, errors.Code(err),
		"a capability limit must be IncompleteChain, not %v", errors.Code(err))
	require.Contains(t, err.Error(), "retained range",
		"the refusal does not name the window: %v", err)
}

// --- reading the network the way production does ---------------------------

// clientFor is the real JSON-RPC client over HTTP against the node's own API,
// wrapped in Querier2 for the typed helpers. Querier2 adds no behaviour of its
// own: it marshals the same queries and unmarshals the same records.
func clientFor(t *testing.T, addr string) apiv3.Querier2 {
	t.Helper()
	return apiv3.Querier2{Querier: jsonrpc.NewClient(addr)}
}

type anchoredBlock struct {
	block           uint64
	stateTreeAnchor [32]byte
}

// anchorsForBVN1 reads the BVN's anchored blocks and their state tree anchors
// out of the Directory's anchor pool, exactly as pull/verify.go does when a
// joining node builds the set of roots it trusts: the pool's main chain,
// expanded, every entry whose body is an anchor.
//
// It is deliberately not read from the BVN's own database. A partition's root
// is proven by an anchor PRODUCED by it, which lives on the receiving
// partition -- reading it from the BVN would be reading the peer's own word
// for its own root (executor.md, "Sync", §1).
func anchorsForBVN1(t *testing.T, c apiv3.Querier2) []anchoredBlock {
	t.Helper()
	ctx := context.Background()
	pool := protocol.DnUrl().JoinPath(protocol.AnchorPool)

	count, expand := uint64(200), true
	page, err := c.QueryMainChainEntries(ctx, pool, &apiv3.ChainQuery{
		Name:  "main",
		Range: &apiv3.RangeOptions{Start: 0, Count: &count, Expand: &expand},
	})
	require.NoError(t, err, "the Directory's anchor pool could not be read")

	var out []anchoredBlock
	seen := map[uint64]bool{}
	for _, rec := range page.Records {
		if rec.Value == nil || rec.Value.Message == nil || rec.Value.Message.Transaction == nil {
			continue
		}
		body, ok := rec.Value.Message.Transaction.Body.(protocol.AnchorBody)
		if !ok {
			continue
		}
		a := body.GetPartitionAnchor()
		if a == nil || a.Source == nil || !protocol.PartitionUrl("BVN1").Equal(a.Source) {
			continue
		}
		if a.StateTreeAnchor == ([32]byte{}) || seen[a.MinorBlockIndex] {
			continue
		}
		seen[a.MinorBlockIndex] = true
		out = append(out, anchoredBlock{block: a.MinorBlockIndex, stateTreeAnchor: a.StateTreeAnchor})
	}
	return out
}

// AN ACCOUNT'S WHOLE LEAF IS SERVED AS OF THE ANCHORED BLOCK (#4361, owner's
// decision 2026-09-22).
//
// A joiner takes the latest signed anchor for block B, pulls every account as
// of B, proves each by equality with the anchor's StateTreeAnchor, and then
// executes B+1 onward. Proving the main body alone is not enough: the BPT leaf
// is H(main, secondary, chains, pending) (internal/database/observer_prod.go,
// hashState), so a joiner that holds only the body at B cannot rebuild the
// leaf, and a joiner that fills the other three parts in from the peer's
// current state executes B+1 on state no anchor covers.
//
// So every part of the leaf must be served as of B, and this test rebuilds the
// leaf from what was served, by hashState's rules, and checks it three ways
// the server cannot satisfy by copying a label: against the value the page for
// B names, against the path the account's own historical receipt proves to
// root(B), and against the anchor the Directory holds for B.
//
// The accounts are chosen so that every part moves after B:
//
//   - an ADI whose directory gains an account and whose pending list gains a
//     transaction after B, and whose main chain grows with each;
//   - the partition ledger, whose chains change every block and whose
//     secondary state carries the scheduled-events BPT;
//   - the synthetic ledger, whose secondary state carries the delivery
//     queues.
//
// Red at base: the historical answer serves the body and blanks the rest
// (internal/api/v3/querier.go, historicalStateReceipt), and there is no field
// at all for chain states, events, delivery queues or the signature sets of a
// pending transaction. servedLeafParts is the one place that reads a response;
// whatever fields the server adds are read there, and nothing else in this
// test needs to change.
func TestAnAccountsWholeLeafIsServedAsOfTheAnchoredBlock(t *testing.T) {
	c := clientFor(t, startNetsimAndExecute(t))
	submitter := c.Querier.(*jsonrpc.Client)
	ctx := context.Background()

	faucetKey := netsimFaucetKey(t, "node-own-state")
	faucet, err := protocol.LiteTokenAddress(faucetKey[32:], "ACME", protocol.SignatureTypeED25519)
	require.NoError(t, err)

	adi := protocol.AccountUrl("leafcheck")
	book := adi.JoinPath("book")
	page := book.JoinPath("1")
	k1, k2 := newEd25519Key(t), newEd25519Key(t)

	submit := func(what string, sb build.SignatureBuilder) {
		t.Helper()
		env, err := sb.Done()
		require.NoError(t, err, "build %s", what)
		subs, err := submitter.Submit(ctx, env, apiv3.SubmitOptions{})
		require.NoError(t, err, "submit %s", what)
		for _, s := range subs {
			require.Truef(t, s.Success, "submit %s: %v", what, s.Message)
		}
	}
	account := func(u *url.URL) *apiv3.AccountRecord {
		r, err := c.QueryAccount(ctx, u, &apiv3.DefaultQuery{})
		if err != nil {
			return nil
		}
		return r
	}
	waitFor := func(what string, ok func() bool) {
		t.Helper()
		require.Eventuallyf(t, ok, 60*time.Second, 250*time.Millisecond, "never: %s", what)
	}
	ts := func() uint64 { return uint64(time.Now().UnixMicro()) }

	// --- the ADI, with a two-of-two page and one transaction pending on it --

	submit("create the ADI", build.Transaction().For(faucet.RootIdentity()).
		CreateIdentity(adi).WithKeyBook(book).WithKey(k1[32:], protocol.SignatureTypeED25519).
		SignWith(faucet.RootIdentity()).Version(1).Timestamp(ts()).
		Type(protocol.SignatureTypeED25519).PrivateKey(faucetKey))
	waitFor("the ADI exists", func() bool { return account(page) != nil })

	submit("fund the page", build.Transaction().For(faucet).
		AddCredits().To(page).WithOracle(1000).Spend(10).
		SignWith(faucet.RootIdentity()).Version(1).Timestamp(ts()).
		Type(protocol.SignatureTypeED25519).PrivateKey(faucetKey))
	waitFor("the page has credits", func() bool {
		r := account(page)
		p, ok := r.Account.(*protocol.KeyPage)
		return ok && p.CreditBalance > 0
	})

	submit("make the page two-of-two", build.Transaction().For(page).
		UpdateKeyPage().Add().Entry().Key(k2[32:], protocol.SignatureTypeED25519).FinishEntry().FinishOperation().
		SetThreshold(2).
		SignWith(page).Version(1).Timestamp(ts()).
		Type(protocol.SignatureTypeED25519).PrivateKey(k1))
	var pageVersion uint64
	waitFor("the page is two-of-two", func() bool {
		r := account(page)
		p, ok := r.Account.(*protocol.KeyPage)
		if ok {
			pageVersion = p.Version
		}
		return ok && p.AcceptThreshold == 2
	})

	createOn := func(name string, keys ...[]byte) {
		t.Helper()
		sb := build.Transaction().For(adi).CreateTokenAccount(adi, name).ForToken(protocol.ACME).
			SignWith(page).Version(pageVersion).Timestamp(ts()).
			Type(protocol.SignatureTypeED25519).PrivateKey(keys[0])
		for _, k := range keys[1:] {
			sb = sb.SignWith(page).Version(pageVersion).Timestamp(ts()).
				Type(protocol.SignatureTypeED25519).PrivateKey(k)
		}
		submit("create "+name, sb)
	}
	pendingCount := func() uint64 {
		r := account(adi)
		if r == nil || r.Pending == nil {
			return 0
		}
		return r.Pending.Total
	}
	createOn("held-before", k1)
	waitFor("a transaction is pending on the ADI", func() bool { return pendingCount() == 1 })

	// --- block B: the ADI as it stands now is the ADI as of B ----------------
	//
	// Nothing but this test writes the ADI, so what it reads now is what it
	// was at every block until the next write. B is the first anchored block
	// at or after this point; the ADI's parts at B are these.
	atB := account(adi)
	require.NotNil(t, atB)
	dirAtB, pendingAtB := urlsOf(atB.Directory), txidsOf(atB.Pending)
	require.NotEmpty(t, dirAtB, "the ADI has no directory at B; the test cannot tell a served directory from a blank one")
	require.Len(t, pendingAtB, 1, "the ADI has no pending transaction at B; the test cannot tell a served pending list from a blank one")

	after := ledgerIndexNow(t, c)
	var b anchoredBlock
	waitFor("the Directory anchors a BVN1 block at or after the ADI settled", func() bool {
		for _, a := range latestAnchorsForBVN1(t, c) {
			if a.block >= after {
				b = a
				return true
			}
		}
		return false
	})
	t.Logf("B = %d", b.block)

	// --- after B, every part of every account moves --------------------------

	createOn("created-after", k1, k2)
	createOn("held-after", k1)
	waitFor("the ADI's directory and pending list moved past B", func() bool {
		r := account(adi)
		return r != nil && len(urlsOf(r.Directory)) > len(dirAtB) && pendingCount() == 2
	})

	// BVN1's synthetic ledger moves only when BVN1 produces a synthetic
	// transaction for another partition, so buy credits for a Directory page:
	// the deposit is appended to BVN1's synthetic chain for the Directory.
	part := protocol.PartitionUrl("BVN1")
	synthChains := func() map[string]uint64 {
		cs, err := c.QueryAccountChains(ctx, part.JoinPath(protocol.Synthetic), &apiv3.ChainQuery{})
		require.NoError(t, err)
		m := map[string]uint64{}
		for _, r := range cs.Records {
			m[r.Name] = r.Count
		}
		return m
	}
	synthBefore := synthChains()
	submit("buy credits for a Directory page", build.Transaction().For(faucet).
		AddCredits().To(protocol.DnUrl().JoinPath(protocol.Operators, "1")).WithOracle(1000).Spend(1).
		SignWith(faucet.RootIdentity()).Version(1).Timestamp(ts()).
		Type(protocol.SignatureTypeED25519).PrivateKey(faucetKey))
	waitFor("BVN1's synthetic ledger moved past B", func() bool {
		for name, n := range synthChains() {
			if n > synthBefore[name] {
				return true
			}
		}
		return false
	})
	waitFor("the partition has moved past B", func() bool { return ledgerIndexNow(t, c) > b.block+3 })

	accounts := []*url.URL{adi, part.JoinPath(protocol.Ledger), part.JoinPath(protocol.Synthetic)}

	// The page for B names every leaf of B's tree, by value.
	pr, err := c.Query(ctx, part, &apiv3.BptPageQuery{Count: 4096, ForHeight: b.block})
	require.NoError(t, err, "the BPT page for block %d was refused", b.block)
	pageAtB, ok := pr.(*apiv3.BptPageRecord)
	require.True(t, ok)
	require.True(t, pageAtB.Done, "the page for block %d did not exhaust the tree", b.block)
	require.Equal(t, b.stateTreeAnchor, pageAtB.BptRoot)

	for _, u := range accounts {
		t.Run(u.ShortString(), func(t *testing.T) {
			r, err := c.QueryAccount(ctx, u, &apiv3.DefaultQuery{
				IncludeReceipt: &apiv3.ReceiptOptions{ForHeight: b.block},
			})
			require.NoErrorf(t, err, "%v was refused at block %d", u, b.block)
			require.NotNil(t, r.Receipt)
			require.Equal(t, b.stateTreeAnchor[:], r.Receipt.Anchor, "the receipt does not end at root(B)")
			require.True(t, r.Receipt.Validate(nil), "the receipt does not verify offline")
			require.True(t, r.Receipt.StartsAtMainState, "the receipt does not start at the main state, so the leaf's parts cannot be told apart")

			leafAtB, ok := leafFor(pageAtB, u)
			require.Truef(t, ok, "the page for block %d does not name %v", b.block, u)

			now, err := c.QueryAccount(ctx, u, &apiv3.DefaultQuery{IncludeReceipt: &apiv3.ReceiptOptions{ForAny: true}})
			require.NoError(t, err)

			// EACH PART IS ITS VALUE AT B, NOT NOW, where the test knows B's
			// value by another route.
			if u.Equal(adi) {
				assert.Equalf(t, dirAtB, urlsOf(r.Directory),
					"the directory served for block %d is not the directory at that block: the server must serve the Directory as of B", b.block)
				assert.NotEqual(t, urlsOf(now.Directory), urlsOf(r.Directory),
					"the directory served for block %d is the current one", b.block)
				assert.Equalf(t, pendingAtB, txidsOf(r.Pending),
					"the pending list served for block %d is not the pending list at that block: the server must serve Pending as of B", b.block)
				assert.NotEqual(t, txidsOf(now.Pending), txidsOf(r.Pending),
					"the pending list served for block %d is the current one", b.block)
			}

			// THE CLAIM. What was served rebuilds, by hashState's rules, the
			// leaf of block B.
			parts := servedLeafParts(r)
			require.Emptyf(t, parts.missing,
				"%v at block %d: the answer does not carry every part of the leaf, so a joiner cannot rebuild it; the server must serve, as of B: %s",
				u, b.block, strings.Join(parts.missing, "; "))

			body, err := r.Account.MarshalBinary()
			require.NoError(t, err)
			mainHash := sha256.Sum256(body)
			leaf := dbmerkle.Hasher{mainHash[:], parts.secondary, parts.chains, parts.pending}.MerkleHash()

			assert.Equalf(t, leafAtB.ValueHash[:], leaf,
				"%v's leaf rebuilt from the answer for block %d is not the leaf block %d's page names", u, b.block, b.block)
			assert.Truef(t, receiptPassesThrough(&r.Receipt.Receipt, *(*[32]byte)(leaf)),
				"%v's leaf rebuilt from the answer for block %d is not on the path its receipt proves to root(B)", u, b.block)

			// And the rebuilt parts are B's and not now's: the second half of
			// the leaf (chains and pending) has moved since B on every one of
			// these accounts, and the current receipt says what it is now.
			require.GreaterOrEqual(t, len(now.Receipt.Entries), 2)
			assert.NotEqualf(t, now.Receipt.Entries[1].Hash, dbmerkle.Hasher{parts.chains, parts.pending}.MerkleHash(),
				"%v's chains and pending served for block %d are the current ones", u, b.block)
		})
	}
}

// leafParts are the three parts of an account's BPT leaf beside its main
// state, each as hashState adds it, or the names of what the answer did not
// carry.
type leafParts struct {
	secondary, chains, pending []byte
	missing                    []string
}

// servedLeafParts rebuilds the parts of a leaf from a served account record,
// by the rules of internal/database/observer_prod.go. It is the only place
// that reads the response, so a field the server adds is read here.
//
// A nil range is not an empty one: every current answer sets Directory (on an
// identity or key book) and Pending, so nil means the server did not say.
func servedLeafParts(r *apiv3.AccountRecord) leafParts {
	var p leafParts
	u := r.Account.GetUrl()
	_, isPartition := protocol.ParsePartitionUrl(u)

	// hashSecondaryState
	var secondary dbmerkle.Hasher
	var dir dbmerkle.Hasher
	switch r.Account.Type() {
	case protocol.AccountTypeIdentity, protocol.AccountTypeKeyBook:
		if r.Directory == nil {
			p.missing = append(p.missing, "the directory")
		}
	}
	if r.Directory != nil {
		for _, v := range r.Directory.Records {
			dir.AddUrl(v.Value)
		}
	}
	secondary.AddValue(dir)
	l := r.Leaf
	if l == nil {
		p.missing = append(p.missing, "the rest of the leaf")
		l = new(apiv3.AccountLeaf)
	}
	if isPartition && u.PathEqual(protocol.Ledger) && l.EventsRoot != [32]byte{} {
		secondary.AddHash2(l.EventsRoot)
	}
	if isPartition && u.PathEqual(protocol.Synthetic) && len(l.LocalDeliveryQueue)+len(l.CascadeDeliveryQueue) > 0 {
		var q dbmerkle.Hasher
		for _, id := range l.LocalDeliveryQueue {
			q.AddUrl(id.AsUrl())
		}
		for _, id := range l.CascadeDeliveryQueue {
			q.AddUrl(id.AsUrl())
		}
		secondary.AddValue(q)
	}
	p.secondary = secondary.MerkleHash()

	// hashChains
	var chains dbmerkle.Hasher
	for _, c := range l.Chains {
		st := &dbmerkle.State{Count: int64(c.Count), Pending: c.State}
		if st.Count == 0 {
			chains.AddHash(new([32]byte))
		} else {
			chains.AddHash((*[32]byte)(st.Anchor()))
		}
	}
	p.chains = chains.MerkleHash()

	// hashPending
	if r.Pending == nil {
		p.missing = append(p.missing, "the pending list")
		return p
	}
	var pending dbmerkle.Hasher
	addSets := func(s *apiv3.PendingTransactionSets) {
		for _, sig := range s.ValidatorSignatures {
			pending.AddHash((*[32]byte)(sig.Hash()))
		}
		for _, h := range s.Payments {
			pending.AddHash2(h)
		}
		for _, v := range s.Votes {
			b, _ := v.MarshalBinary()
			pending.AddHash2(sha256.Sum256(b))
		}
		for _, v := range s.Signatures {
			b, _ := v.MarshalBinary()
			pending.AddHash2(sha256.Sum256(b))
		}
	}
	for _, s := range l.Pending {
		if len(s.V1Hashes) > 0 {
			for _, h := range s.V1Hashes {
				pending.AddHash2(h)
			}
		} else {
			pending.AddTxID(s.TxID)
		}
		addSets(s)
	}
	for _, s := range l.BookPending {
		addSets(s)
	}
	p.pending = pending.MerkleHash()
	return p
}

func urlsOf(r *apiv3.RecordRange[*apiv3.UrlRecord]) []string {
	var out []string
	if r != nil {
		for _, v := range r.Records {
			out = append(out, v.Value.String())
		}
	}
	return out
}

func txidsOf(r *apiv3.RecordRange[*apiv3.TxIDRecord]) []string {
	var out []string
	if r != nil {
		for _, v := range r.Records {
			out = append(out, v.Value.String())
		}
	}
	return out
}

// netsimFaucetKey derives the faucet key a netsim started with the given P2P
// seed funds at genesis, by the netsim's own derivation.
func netsimFaucetKey(t *testing.T, seed string) []byte {
	t.Helper()
	cfg := &Config{P2P: &P2P{Key: &PrivateKeySeed{Seed: record.NewKey(seed)}}}
	k, err := (&NetSimConfiguration{}).generateKey(&Instance{logger: slog.Default(), context: context.Background()}, cfg, "faucet")
	require.NoError(t, err)
	return k
}

func ledgerIndexNow(t *testing.T, c apiv3.Querier2) uint64 {
	t.Helper()
	l := new(protocol.SystemLedger)
	_, err := c.QueryAccountAs(context.Background(), protocol.PartitionUrl("BVN1").JoinPath(protocol.Ledger), nil, &l)
	require.NoError(t, err)
	return l.Index
}

// latestAnchorsForBVN1 is anchorsForBVN1 read from the end of the pool, so a
// network that has run long enough to fill the first page is still read.
func latestAnchorsForBVN1(t *testing.T, c apiv3.Querier2) []anchoredBlock {
	t.Helper()
	pool := protocol.DnUrl().JoinPath(protocol.AnchorPool)
	count, expand := uint64(100), true
	page, err := c.QueryMainChainEntries(context.Background(), pool, &apiv3.ChainQuery{
		Name:  "main",
		Range: &apiv3.RangeOptions{Count: &count, Expand: &expand, FromEnd: true},
	})
	require.NoError(t, err, "the Directory's anchor pool could not be read")

	var out []anchoredBlock
	for _, rec := range page.Records {
		if rec.Value == nil || rec.Value.Message == nil || rec.Value.Message.Transaction == nil {
			continue
		}
		body, ok := rec.Value.Message.Transaction.Body.(protocol.AnchorBody)
		if !ok {
			continue
		}
		a := body.GetPartitionAnchor()
		if a == nil || a.Source == nil || !protocol.PartitionUrl("BVN1").Equal(a.Source) || a.StateTreeAnchor == ([32]byte{}) {
			continue
		}
		out = append(out, anchoredBlock{block: a.MinorBlockIndex, stateTreeAnchor: a.StateTreeAnchor})
	}
	return out
}
