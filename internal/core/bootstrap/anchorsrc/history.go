// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package anchorsrc

import (
	"bytes"
	"context"
	"math/bits"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

const (
	// maxHistoryEntries bounds how many bpt entries one proof reads between
	// the base anchor's root and the root being proven. The base is the
	// latest verified anchor below the block the root was served at, so the
	// gap is the non-empty blocks between two anchors: one under load, a
	// heartbeat's worth idle. A gap this wide is a base too old to use, and
	// the pass is fetched again against the next anchor.
	maxHistoryEntries = 1 << 14

	// historyPage is how many bpt entries are asked for per call. The
	// querier serves at most 100 (api/v3, maxPageSize).
	historyPage = 100
)

// anchorAt is one verified anchor with the block it was produced for.
type anchorAt struct {
	verifiedAnchor
	block uint64
}

// ProveRoot reports whether root is a BPT root of the producer. Two things
// prove it, and nothing else:
//
//   - it EQUALS the StateTreeAnchor of an anchor a quorum of the producer's
//     validators signed and this source verified; or
//   - the history from such a StateTreeAnchor to root, read from the
//     producer's bpt chain, hashes into the root chain anchor a LATER verified
//     anchor signs.
//
// A signed anchor for block N carries the BPT root block N committed
// (crosschain/anchoring.go; pinned by
// TestAnAnchorsStateTreeAnchorIsTheRootOfItsBlock), which is the root a
// peer's state is CURRENT at while its ledger says N. Block N+1 records that
// root on the ledger's bpt chain and anchors the bpt chain into the root chain
// (block_end.go), and the anchor of any block after N signs the root chain's
// anchor and its height. So the roots between two anchors are the bpt
// entries between their StateTreeAnchors, and a root served at a block no
// anchor was sent for is proven by the anchors on either side of it (Paul,
// 2026-09-22: "the anchor + the history is everything for the current block
// every block").
//
// **The proof reads no index and takes no number from a peer.** With the
// latest verified anchor L and the latest verified anchor B below the served
// block, both receipts are asked for at L's signed root chain height, and
// that height alone decides which step of each is a root chain entry
// (splitAtLeaf): a receipt's shape is the path from the root chain's anchor
// down to one of its entries, and a path from the root reaches one node. B's
// root is the last bpt entry at its own anchoring, so the steps below its
// root chain entry are the bpt chain's peaks at that height, and they rebuild
// the bpt chain's merkle state there. The bpt entries from B's to root are
// read and appended, root must be the last, and the state's anchor must be
// the root chain entry root's own receipt enters at. Every hash in that
// chain of reasoning is a node on a receipt to the signed anchor, so a peer
// that names another entry index, serves other entries, or serves a
// transaction's true receipt as a root's produces a fresh hash the root chain
// does not hold (TestATransactionsReceiptIsRefusedThoughThePeerForgesTheBptIndex,
// TestAReceiptThatIsNotTheBptChainsIsRefused). The entry indices the peer
// reports shape the rebuild and the range asked for; they enter no
// conclusion. What is left open: an interior node of the bpt chain, whose
// leaves are true roots, can pass for an entry when the peer chooses the
// range (DIFFERENCES.md E11).
//
// q reaches the producer's peers, never this node (#4303). It is used only
// when the history is read; a root proven by equality, and a wait, need no
// peer. servedAt is the block the peer said the state was served at, zero
// if it did not say. It chooses the base and tells a wait from an error.
//
// False with no error is a wait: no verified anchor carries the root yet, or
// none after the served block is verified yet, and the next one may. An error
// is a root this source will not prove as things stand, and what was fetched
// at it is fetched again: the root is empty (the state was served with no
// receipt), no peer answered for the history, the history does not hash to a
// signed anchor, or the history has PASSED it — an anchor after the served
// block is verified, and the bpt chain does not record it.
func (s *Source) ProveRoot(ctx context.Context, q api.Querier, root [32]byte, servedAt uint64) (bool, error) {
	if root == ([32]byte{}) {
		return false, errors.BadRequest.With("there is no root to prove: the state was served without a receipt")
	}

	proven, latest, base, err := s.locate(ctx, root, servedAt)
	switch {
	case err != nil:
		return false, err
	case proven:
		return true, nil
	case latest == nil:
		return false, nil // Nothing is verified yet
	case servedAt != 0 && latest.block <= servedAt:
		// The served block's root reaches the root chain in the block
		// after it, and no verified anchor signs that far yet.
		return false, nil
	case base == nil:
		if servedAt == 0 {
			return false, nil
		}
		return false, errors.NotFound.WithFormat(
			"the history has passed %x: it was served at block %d, the anchor of block %d is verified, and no verified anchor precedes it to read the history from",
			root[:4], servedAt, latest.block)
	case q == nil:
		return false, errors.BadRequest.With("anchorsrc.ProveRoot: a querier is required to read the history")
	}
	return s.proveByHistory(ctx, api.Querier2{Querier: q}, root, servedAt, *latest, *base)
}

// locate answers from the verified anchors: whether root is one of their
// StateTreeAnchors, the latest of them, and the latest below the served block
// (below the latest, when the served block is not known). The pool is read
// when the root is not already proven.
func (s *Source) locate(ctx context.Context, root [32]byte, servedAt uint64) (proven bool, latest, base *anchorAt, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	want := strings.ToLower(s.Producer.String())
	search := func() bool {
		for k, a := range s.roots {
			if k.partition != want {
				continue
			}
			if a.root == root {
				return true
			}
			if latest == nil || k.block > latest.block {
				latest = &anchorAt{a, k.block}
			}
		}
		return false
	}
	if search() {
		return true, nil, nil, nil
	}
	if err := s.readLocked(ctx); err != nil {
		return false, nil, nil, errors.UnknownError.Wrap(err)
	}
	latest = nil
	if search() {
		return true, nil, nil, nil
	}
	if latest == nil {
		return false, nil, nil, nil
	}
	below := servedAt
	if below == 0 {
		below = latest.block
	}
	for k, a := range s.roots {
		if k.partition != want || k.block >= below {
			continue
		}
		if base == nil || k.block > base.block {
			base = &anchorAt{a, k.block}
		}
	}
	return false, latest, base, nil
}

// proveByHistory reads the bpt chain from base's root to root and holds what
// it reads to latest's signed root chain anchor. See ProveRoot.
func (s *Source) proveByHistory(ctx context.Context, q api.Querier2, root [32]byte, servedAt uint64, latest, base anchorAt) (bool, error) {
	ledger := s.Producer.JoinPath(protocol.Ledger)
	ask := func(entry [32]byte) (*api.ChainEntryRecord[api.Record], error) {
		return q.QueryChainEntry(ctx, ledger, &api.ChainQuery{
			Name:           "bpt",
			Entry:          entry[:],
			IncludeReceipt: &api.ReceiptOptions{ForHeight: latest.rootIndex},
		})
	}

	// The base: a signed root, the last bpt entry at its own anchoring.
	baseRec, err := ask(base.root)
	if err != nil {
		return false, errors.UnknownError.WithFormat(
			"read the bpt entry of block %d's root from the producer's peers: %w", base.block, err)
	}
	baseStep, _, baseLeaf, err := rootChainEntryOf(baseRec, base.root, latest)
	if err != nil {
		return false, err
	}
	state, err := stateAtLastEntry(baseRec.Index, &baseRec.Receipt.Receipt, baseStep)
	if err != nil {
		return false, err
	}

	// The root: recorded after the base, and its receipt enters the root
	// chain later than the base's.
	rootRec, err := ask(root)
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.NotFound):
		if servedAt == 0 {
			return false, nil // Not recorded yet: the next block records it
		}
		return false, errors.NotFound.WithFormat(
			"the history has passed %x: it was served at block %d, the anchor of block %d is verified, and the bpt chain does not record it",
			root[:4], servedAt, latest.block)
	default:
		return false, errors.UnknownError.WithFormat("read bpt entry %x from the producer's peers: %w", root[:4], err)
	}
	rootStep, rootHash, rootLeaf, err := rootChainEntryOf(rootRec, root, latest)
	if err != nil {
		return false, err
	}
	switch {
	case rootRec.Index <= baseRec.Index || rootLeaf <= baseLeaf:
		return false, errors.Conflict.WithFormat(
			"bpt entry %x is served as entry %d anchored at root chain entry %d, and block %d's root is entry %d anchored at %d: the root is not after the base",
			root[:4], rootRec.Index, rootLeaf, base.block, baseRec.Index, baseLeaf)
	case rootStep != bits.OnesCount64(rootRec.Index):
		return false, errors.Conflict.WithFormat(
			"the receipt for bpt entry %x has %d steps below the root chain, and entry %d of a bpt chain, last at its anchoring, has %d",
			root[:4], rootStep, rootRec.Index, bits.OnesCount64(rootRec.Index))
	case rootRec.Index-baseRec.Index > maxHistoryEntries:
		return false, errors.Conflict.WithFormat(
			"the history from block %d's root to %x is %d bpt entries, more than the %d one proof reads",
			base.block, root[:4], rootRec.Index-baseRec.Index, maxHistoryEntries)
	}

	// The entries between, appended in order; root must be the last.
	var last [32]byte
	for next := baseRec.Index + 1; next <= rootRec.Index; {
		count := rootRec.Index - next + 1
		if count > historyPage {
			count = historyPage
		}
		page, err := q.QueryChainEntries(ctx, ledger, &api.ChainQuery{
			Name:  "bpt",
			Range: &api.RangeOptions{Start: next, Count: &count},
		})
		if err != nil {
			return false, errors.UnknownError.WithFormat(
				"read bpt entries %d.. from the producer's peers: %w", next, err)
		}
		if page == nil || len(page.Records) == 0 {
			return false, errors.Conflict.WithFormat(
				"the bpt chain ends before entry %d, where the peer placed %x", next, root[:4])
		}
		for i, rec := range page.Records {
			if rec == nil || rec.Index != next+uint64(i) {
				return false, errors.Conflict.WithFormat("bpt entries %d.. were served out of order", next)
			}
			state.AddEntry(rec.Entry[:])
			last = rec.Entry
		}
		next += uint64(len(page.Records))
	}
	if last != root {
		return false, errors.Conflict.WithFormat(
			"bpt entry %d is %x, and %x was served as that entry", rootRec.Index, last[:4], root[:4])
	}

	// The rebuilt chain's anchor is the root chain entry root's receipt
	// enters at, or the entries served are not the bpt chain's.
	if !bytes.Equal(state.Anchor(), rootHash) {
		return false, errors.Conflict.WithFormat(
			"the bpt chain rebuilt from block %d's root to %x does not anchor at the root chain entry %x's receipt enters at: it is not the bpt chain's history",
			base.block, root[:4], root[:4])
	}
	return true, nil
}

// rootChainEntryOf checks a bpt entry's receipt against the latest verified
// anchor and finds the step at which it is an entry of the root chain: that
// step, the hash there, and the entry's index in the root chain.
func rootChainEntryOf(rec *api.ChainEntryRecord[api.Record], entry [32]byte, latest anchorAt) (int, []byte, uint64, error) {
	if rec == nil || rec.Receipt == nil {
		return 0, nil, 0, errors.Conflict.WithFormat("a peer served the bpt entry %x with no receipt", entry[:4])
	}
	r := &rec.Receipt.Receipt
	switch {
	case !r.Validate(nil):
		return 0, nil, 0, errors.Conflict.WithFormat("the receipt for bpt entry %x does not validate", entry[:4])
	case !bytes.Equal(r.Start, entry[:]):
		return 0, nil, 0, errors.Conflict.WithFormat("the receipt for bpt entry %x starts at %x", entry[:4], r.Start)
	case !bytes.Equal(r.Anchor, latest.rootAnchor[:]):
		return 0, nil, 0, errors.Conflict.WithFormat(
			"the receipt for bpt entry %x ends at %x, and block %d's anchor signs the root chain anchor %x",
			entry[:4], r.Anchor, latest.block, latest.rootAnchor[:4])
	}
	step, hash, index, ok := splitAtLeaf(r, latest.rootIndex+1)
	if !ok {
		return 0, nil, 0, errors.Conflict.WithFormat(
			"no step of the receipt for bpt entry %x is an entry of a root chain of %d", entry[:4], latest.rootIndex+1)
	}
	return step, hash, index, nil
}

// stateAtLastEntry rebuilds a chain's merkle state at the anchoring where
// entry index was its last entry, from the first steps of that entry's
// receipt.
//
// The last entry of a chain climbs with every other hash on its left: one
// step per trailing one of its index, up to the tree it completes, then one
// per larger tree, smallest first — which is how the state's anchor folds its
// pending hashes. So the running hash after the trailing ones is the pending
// hash at that level, and each step after it is the pending hash at the next
// set bit of the count. A step on the right, or a step count the index does
// not account for, is a receipt that is not a last entry's.
func stateAtLastEntry(index uint64, r *merkle.Receipt, steps int) (*merkle.State, error) {
	if bits.OnesCount64(index) != steps {
		return nil, errors.Conflict.WithFormat(
			"the receipt for bpt entry %x has %d steps below the root chain, and entry %d of a bpt chain, last at its anchoring, has %d",
			r.Start[:4], steps, index, bits.OnesCount64(index))
	}
	notLast := func() error {
		return errors.Conflict.WithFormat("the receipt for bpt entry %x is not that of a chain's last entry", r.Start[:4])
	}
	count := index + 1
	ones := bits.TrailingZeros64(^index)
	running := append([]byte(nil), r.Start...)
	k := 0
	for ; k < ones; k++ {
		if r.Entries[k].Right {
			return nil, notLast()
		}
		running = applyEntry(r.Entries[k], running)
	}
	pending := make([][]byte, bits.Len64(count))
	pending[ones] = running
	for b := ones + 1; b < len(pending); b++ {
		if count&(1<<b) == 0 {
			continue
		}
		if r.Entries[k].Right {
			return nil, notLast()
		}
		pending[b] = append([]byte(nil), r.Entries[k].Hash...)
		k++
	}
	return &merkle.State{Count: int64(count), Pending: pending}, nil
}
