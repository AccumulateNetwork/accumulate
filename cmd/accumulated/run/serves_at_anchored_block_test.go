// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"crypto/sha256"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	apiv3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
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

	// ALL THREE HOT ACCOUNTS, not just the ledger. The done-when names
	// `<partition>/ledger`, `<partition>/synthetic` and `<partition>/anchors`,
	// and the two sequence ledgers are where the byte-for-byte rule was broken
	// before: `Received` is derived from staging and was filled in on read,
	// which left every restarting node unable to pull `<partition>/anchors`
	// from anybody (#4295). A body with one synthesised field does not hash to
	// the leaf its own receipt proves.
	part := protocol.PartitionUrl("BVN1")
	for _, name := range []string{protocol.Ledger, protocol.Synthetic, protocol.AnchorPool} {
		t.Run(name, func(t *testing.T) {
			servedAsOfAnchoredBlocks(t, c, ctx, part.JoinPath(name), anchors)
		})
	}
}

// preImageMissing is the refusal #4388 produces: the node holds the account's
// BPT entry for the block but not the state behind it.
const preImageMissing = "but not the state behind it"

// servedAsOfAnchoredBlocks is the body of the test above, run once per hot
// account.
func servedAsOfAnchoredBlocks(t *testing.T, c apiv3.Querier2, ctx context.Context, account *url.URL, anchors []anchoredBlock) {
	t.Helper()
	ledger := account
	served := 0
	preImage := 0
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
			if strings.Contains(err.Error(), preImageMissing) {
				preImage++
			}
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

		// THE BODY IS CHECKED WHATEVER THE PEER SAYS. This used to sit inside
		// `if r.Receipt.StartsAtMainState`, which let the peer's own flag
		// decide whether its body was checked: a mutation serving the CURRENT
		// body with the flag off passed all five daemon tests. A historical
		// answer on this line is coherent or it is a refusal, so the flag is
		// required and the body is compared, unconditionally.
		require.Truef(t, r.Receipt.StartsAtMainState,
			"the receipt for block %d does not start at the main state, so the body served with it cannot be checked", a.block)
		body, err := r.Account.MarshalBinary()
		require.NoError(t, err)
		h := sha256.Sum256(body)
		require.Equalf(t, h[:], r.Receipt.Start,
			"the receipt for block %d does not start at the body it was served with", a.block)
		served++
		lastServed = a.block
	}
	// #4388, and it un-skips itself. An account whose only changes inside the
	// retained window fall AFTER the block asked about has no retained body
	// for that block: retention writes the state after a change, so there is
	// nothing for a block before the account's first change in the window. The
	// newest anchored block is several behind the newest block, so an account
	// that changed only in those last few blocks — which on a young network is
	// `<partition>/synthetic` — cannot be served at the block the join asks
	// about. The skip is conditional on seeing exactly that refusal and
	// nothing else, so the day #4388 is fixed this runs and asserts.
	if served == 0 && preImage == len(anchors) {
		t.Skipf("#4388: %v changed inside the window only after every anchored block, "+
			"so no body was retained for any of them; %d of %d refusals were that and nothing else",
			ledger, preImage, len(anchors))
	}
	require.NotZero(t, served, "not one anchored block could be served")
	t.Logf("%v: served %d of %d anchored blocks, last %d", ledger, served, len(anchors), lastServed)
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
		// Receipt.Contains is the library's own "is this value on the path",
		// rather than a fold written out here: a check the client makes with
		// the code it already has is the check a joining node would make.
		require.Truef(t,
			acct.Receipt.Receipt.Contains(&merkle.Receipt{Start: leaf.ValueHash[:]}),
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

// THE RETENTION WINDOW CLOSES, AND ITS FLOOR MOVES.
//
// The refusal is IncompleteChain (414), a capability limit -- NOT NotFound,
// which this path reserves for "this node's index has no record of the account
// at that height". The issue's done-when says "NotFound that says so"; the
// four-status split is main's and it is worth keeping, so this is a deliberate
// difference and it is recorded on #4361.
//
// # Why this asks about a block the window has DROPPED, not about block 1
//
// The first version of this test asked at block 1 and required the refusal to
// name a range. Block 1 is refused at every depth with the identical message
// -- retention first runs at block 2, because a first write supersedes nothing,
// so block 1 is below the HORIZON whatever the window is. The reviewer's
// mutation W, which stops the floor advancing so the window never closes,
// passed it 7/7 along with every other test. A window that grows without limit
// is the storage failure the depth exists to prevent, and nothing would have
// caught it until a soak did, twelve hours late.
//
// So it asks about the floor: at depth 4 the window really has dropped blocks,
// the floor must have moved off the horizon, the range must be about as wide as
// the depth, and a block one below the floor must be refused. That last is
// race-free on a moving network because the floor only ever rises.
func TestTheRetentionWindowClosesAndItsFloorMoves(t *testing.T) {
	const depth = uint64(4)
	d := depth
	c := clientFor(t, startNetsimAndExecuteWith(t, &d))
	ctx := context.Background()

	ledger := protocol.PartitionUrl("BVN1").JoinPath(protocol.Ledger)

	now, err := c.QueryAccount(ctx, ledger, &apiv3.DefaultQuery{
		IncludeReceipt: &apiv3.ReceiptOptions{ForAny: true},
	})
	require.NoError(t, err, "the current-state path must keep working")
	require.NotNil(t, now.Receipt)

	// Block 1 is indexed -- the node has executed dozens of blocks since -- and
	// is below both the horizon and the floor, so it names the window.
	_, err = c.QueryAccount(ctx, ledger, &apiv3.DefaultQuery{
		IncludeReceipt: &apiv3.ReceiptOptions{ForHeight: 1},
	})
	require.Error(t, err, "a block far below the retention horizon was answered")
	require.Equalf(t, errors.IncompleteChain, errors.Code(err),
		"a capability limit must be IncompleteChain, not %v", errors.Code(err))
	require.Contains(t, err.Error(), "retained range",
		"the refusal does not name the window: %v", err)

	earliest, latest := retainedRange(t, err)
	t.Logf("depth %d: retained range [%d, %d]", depth, earliest, latest)

	// THE CLAIM. The floor has left the horizon, so blocks have been dropped,
	// and the range is about as wide as the depth rather than the whole chain.
	require.Greaterf(t, earliest, uint64(2),
		"the floor is still at the first retained block, so nothing has been dropped: the window at depth %d never closed", depth)
	require.GreaterOrEqual(t, latest, earliest)
	require.LessOrEqualf(t, latest-earliest, depth+2,
		"the range [%d, %d] is wider than a depth of %d allows", earliest, latest, depth)

	// And the range is predictive at its floor: one below it is refused, and
	// the refusal names the window rather than reporting the account missing.
	_, err = c.QueryAccount(ctx, ledger, &apiv3.DefaultQuery{
		IncludeReceipt: &apiv3.ReceiptOptions{ForHeight: earliest - 1},
	})
	require.Errorf(t, err, "block %d, one below the advertised floor, was answered", earliest-1)
	require.Equalf(t, errors.IncompleteChain, errors.Code(err),
		"block %d was refused as %v rather than as a capability limit", earliest-1, errors.Code(err))

	// And predictive at its top: the newest block it advertises is answerable.
	// Re-read the range each try, because the network is still moving and the
	// floor rises under the test.
	var servedAt uint64
	for i := 0; i < 5 && servedAt == 0; i++ {
		_, e := c.QueryAccount(ctx, ledger, &apiv3.DefaultQuery{
			IncludeReceipt: &apiv3.ReceiptOptions{ForHeight: latest},
		})
		if e == nil {
			servedAt = latest
			break
		}
		_, err := c.QueryAccount(ctx, ledger, &apiv3.DefaultQuery{
			IncludeReceipt: &apiv3.ReceiptOptions{ForHeight: 1},
		})
		require.Error(t, err)
		earliest, latest = retainedRange(t, err)
	}
	require.NotZerof(t, servedAt,
		"the newest block the node advertises was not answerable, so the range over-promises")
	t.Logf("served the top of the window at block %d", servedAt)
}

// retainedRange reads the window out of a refusal. The range is advertised
// nowhere else on this line -- main reports it on ConsensusStatus and that was
// not ported -- so the message is the only place a client can learn it, which
// is itself worth pinning.
func retainedRange(t *testing.T, err error) (earliest, latest uint64) {
	t.Helper()
	m := regexp.MustCompile(`retained range is \[(\d+), (\d+)\]`).FindStringSubmatch(err.Error())
	require.Lenf(t, m, 3, "the refusal does not name a range a client could read: %v", err)
	e, cerr := strconv.ParseUint(m[1], 10, 64)
	require.NoError(t, cerr)
	l, cerr := strconv.ParseUint(m[2], 10, 64)
	require.NoError(t, cerr)
	return e, l
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
