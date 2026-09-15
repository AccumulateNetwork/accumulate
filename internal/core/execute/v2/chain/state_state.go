// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"math/big"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/internal"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type ProcessTransactionState struct {
	ProducedTxns       []*protocol.Transaction
	AdditionalMessages []messaging.Message
	ChainUpdates       ChainUpdates
	MakeMajorBlock     uint64
	MakeMajorBlockTime time.Time
	ReceivedAnchors    []*ReceivedAnchor
	AcmeBurnt          big.Int
	NetworkUpdate      []*protocol.NetworkAccountUpdate
}

type ReceivedAnchor struct {
	Partition string
	Body      protocol.AnchorBody
	Index     int64
}

// DidProduceTxn records a produced transaction.
func (s *ProcessTransactionState) DidProduceTxn(url *url.URL, body protocol.TransactionBody) {
	txn := new(protocol.Transaction)
	txn.Header.Principal = url
	txn.Body = body
	s.ProducedTxns = append(s.ProducedTxns, txn)
}

func (s *ProcessTransactionState) DidReceiveAnchor(partition string, body protocol.AnchorBody, index int64) {
	s.ReceivedAnchors = append(s.ReceivedAnchors, &ReceivedAnchor{partition, body, index})
}

// ProcessNetworkMaintenanceOp queues a [internal.NetworkMaintenanceOp] message for processing
// after the current bundle.
func (s *ProcessTransactionState) ProcessNetworkMaintenanceOp(cause *url.TxID, op protocol.NetworkMaintenanceOperation) {
	s.AdditionalMessages = append(s.AdditionalMessages, &internal.NetworkMaintenanceOp{
		Cause:     cause,
		Operation: op,
	})
}

// ProcessNetworkUpdate queues a [internal.NetworkUpdate] message for processing
// after the current bundle.
func (s *ProcessTransactionState) ProcessNetworkUpdate(cause [32]byte, account *url.URL, body protocol.TransactionBody) {
	s.AdditionalMessages = append(s.AdditionalMessages, &internal.NetworkUpdate{
		Cause:   cause,
		Account: account,
		Body:    body,
	})
}

// ProcessTransaction queues a transaction for processing after the current
// bundle.
func (s *ProcessTransactionState) ProcessTransaction(txid *url.TxID) {
	s.AdditionalMessages = append(s.AdditionalMessages, &internal.MessageIsReady{
		TxID: txid,
	})
}

func (s *ProcessTransactionState) Merge(r *ProcessTransactionState) {
	if r.MakeMajorBlock > 0 {
		s.MakeMajorBlock = r.MakeMajorBlock
		s.MakeMajorBlockTime = r.MakeMajorBlockTime
	}
	s.ProducedTxns = append(s.ProducedTxns, r.ProducedTxns...)
	s.AdditionalMessages = append(s.AdditionalMessages, r.AdditionalMessages...)
	s.ChainUpdates.Merge(&r.ChainUpdates)
	s.ReceivedAnchors = append(s.ReceivedAnchors, r.ReceivedAnchors...)
	s.AcmeBurnt.Add(&s.AcmeBurnt, &r.AcmeBurnt)
	s.NetworkUpdate = append(s.NetworkUpdate, r.NetworkUpdate...)
}

type ChainUpdates struct {
	Entries []*protocol.BlockEntry

	// Hashes holds, for each chain appended to through this record, the
	// hash at the index the block last appended: what the block close needs
	// for the transaction-chain index, known at append and kept so the chain
	// is not read back to recover it (#4245). Keyed by SegmentKey.
	Hashes map[string]ChainEntryHash

	// Segments holds, for each anchor chain this block appended to, the
	// chain's state before the block's first append and the hashes appended
	// since: what a receipt over this block's appends is built from, without
	// reading the chain back (a chain is a sequence of hashes to everything
	// above the merkle library). Keyed by SegmentKey. Only anchor chains:
	// the root chain has its own segment in the block, the synthetic chain
	// its own in the producer cache, and a main chain's receipts are never
	// built here.
	Segments map[string]*merkle.Segment

	// segmentErrs is, per anchor chain, the first span that arrived out of
	// chain order; see addSegment.
	segmentErrs map[string]error

	// byChain is the position in Entries of the first entry for each
	// (account, chain), so AddChainEntry2 finds a chain's entry without
	// scanning the block -- E compares per append was O(E²) per block,
	// 1-3 M URL compares at 500 tps (#4226). The slice is the source of
	// truth: the block rebuilds and sorts it directly, so a hit is checked
	// against the slice and the index is rebuilt when the slice has changed
	// under it (indexed is the length it was built for).
	byChain map[string]int
	indexed int
}

// SegmentKey names a chain for ChainUpdates.Segments and Hashes.
func SegmentKey(account *url.URL, chain string) string {
	return strings.ToLower(account.String()) + ";" + chain
}

// entryFor returns the block entry recorded for a chain, if there is one:
// the first, as the scan it replaces returned.
func (c *ChainUpdates) entryFor(account *url.URL, chain string) (*protocol.BlockEntry, bool) {
	key := SegmentKey(account, chain)
	if c.byChain == nil || c.indexed != len(c.Entries) {
		c.reindex()
	}
	i, ok := c.byChain[key]
	if !ok {
		return nil, false
	}
	if e := c.Entries[i]; e.Chain == chain && e.Account.Equal(account) {
		return e, true
	}
	// The slice was reordered or replaced under the index
	c.reindex()
	if i, ok = c.byChain[key]; ok {
		return c.Entries[i], true
	}
	return nil, false
}

func (c *ChainUpdates) reindex() {
	c.byChain = make(map[string]int, len(c.Entries))
	for i, e := range c.Entries {
		key := SegmentKey(e.Account, e.Chain)
		if _, ok := c.byChain[key]; !ok {
			c.byChain[key] = i
		}
	}
	c.indexed = len(c.Entries)
}

// ChainEntryHash is the hash appended to a chain at an index.
type ChainEntryHash struct {
	Index uint64
	Hash  []byte
}

// EntryHash returns the hash the block appended to the chain at the index, if
// it was appended through this record.
func (c *ChainUpdates) EntryHash(account *url.URL, chain string, index uint64) ([]byte, bool) {
	h, ok := c.Hashes[SegmentKey(account, chain)]
	if !ok || h.Index != index {
		return nil, false
	}
	return h.Hash, true
}

func (c *ChainUpdates) didAppend(account *url.URL, chain string, index uint64, hash []byte) {
	if c.Hashes == nil {
		c.Hashes = map[string]ChainEntryHash{}
	}
	key := SegmentKey(account, chain)
	if cur, ok := c.Hashes[key]; ok && cur.Index > index {
		return
	}
	c.Hashes[key] = ChainEntryHash{Index: index, Hash: hash}
}

func (c *ChainUpdates) Merge(d *ChainUpdates) {
	for _, u := range d.Entries {
		c.DidUpdateChain(u)
	}
	for k, h := range d.Hashes {
		if cur, ok := c.Hashes[k]; ok && cur.Index > h.Index {
			continue
		}
		if c.Hashes == nil {
			c.Hashes = map[string]ChainEntryHash{}
		}
		c.Hashes[k] = h
	}
	for k, seg := range d.Segments {
		c.addSegment(k, seg)
	}
	for k, err := range d.segmentErrs {
		if c.segmentErrs == nil {
			c.segmentErrs = map[string]error{}
		}
		if _, ok := c.segmentErrs[k]; !ok {
			c.segmentErrs[k] = err
		}
	}
}

// addSegment joins one message's span of an anchor chain onto the block's.
//
// Spans arrive in chain order: messages execute in the order staging
// released them, a bundle folds its states in that order (bundleStates),
// and the bundles fold into the block in that order. So a span either
// starts the block's segment of the chain or continues it. Any other span
// means the order was lost upstream -- per-message states were once folded
// in hash order, and the third of three anchors from one partition was
// dropped on the floor, on every node (#4279, run 20260915T042428Z). It is
// refused out loud, not dropped: the block will not build receipts over a
// segment with a hole in it, and the error says where the hole is.
func (c *ChainUpdates) addSegment(k string, seg *merkle.Segment) {
	if c.Segments == nil {
		c.Segments = map[string]*merkle.Segment{}
	}
	cur, ok := c.Segments[k]
	switch {
	case !ok:
		c.Segments[k] = seg
	case seg.First == cur.Last()+1:
		cur.Elements = append(cur.Elements, seg.Elements...)
	default:
		if c.segmentErrs == nil {
			c.segmentErrs = map[string]error{}
		}
		if _, ok := c.segmentErrs[k]; !ok {
			c.segmentErrs[k] = errors.InternalError.WithFormat("chain %s: span [%d, %d] folded after [%d, %d]; appends are out of chain order", k, seg.First, seg.Last(), cur.First, cur.Last())
		}
	}
}

// SegmentError is the first span of the named anchor chain that folded out of
// chain order this block, if any. A receipt must not be built over that
// segment.
//
// Per chain, not per block: the fault belongs to the chain whose segment has
// the hole, and a block carries segments for chains no receipt is built from
// (an anchor's -bpt chain, a partition not in this block's ReceivedAnchors).
// Failing the whole block on any of them turns one unbuildable receipt into a
// partition that cannot close a block at all, which is how #4279 killed the
// Directory in the first place.
func (c *ChainUpdates) SegmentError(key string) error {
	if c.segmentErrs == nil {
		return nil
	}
	return c.segmentErrs[key]
}

// DidUpdateChain records a chain update.
func (c *ChainUpdates) DidUpdateChain(update *protocol.BlockEntry) {
	c.Entries = append(c.Entries, update)
	if c.byChain == nil || c.indexed != len(c.Entries)-1 {
		return // Not built, or stale: the next lookup rebuilds it
	}
	key := SegmentKey(update.Account, update.Chain)
	if _, ok := c.byChain[key]; !ok {
		c.byChain[key] = len(c.Entries) - 1
	}
	c.indexed = len(c.Entries)
}

// DidAddChainEntry records a chain update in the block state.
func (c *ChainUpdates) DidAddChainEntry(batch *database.Batch, u *url.URL, name string, typ protocol.ChainType, entry []byte, index, sourceIndex, sourceBlock uint64) error {
	var update protocol.BlockEntry
	update.Account = u
	update.Chain = name
	update.Index = index
	c.DidUpdateChain(&update)
	c.didAppend(u, name, index, entry)
	return nil
}

// AddChainEntry adds an entry to a chain and records the chain update in the
// block state.
func (u *ChainUpdates) AddChainEntry(batch *database.Batch, chain *database.Chain2, entry []byte, sourceIndex, sourceBlock uint64) error {
	_, err := u.AddChainEntry2(batch, chain, entry, sourceIndex, sourceBlock, true)
	return err
}

func (u *ChainUpdates) AddChainEntry2(batch *database.Batch, chain *database.Chain2, entry []byte, sourceIndex, sourceBlock uint64, unique bool) (int64, error) {
	// Add an entry to the chain
	c, err := chain.Get()
	if err != nil {
		return 0, errors.UnknownError.WithFormat("load %s chain: %w", chain.Name(), err)
	}

	// A transaction appends to a chain once (database spec, "Duplicates are
	// caught at entry"). The state cache appends the transaction hash to
	// every account it writes, and the success path appends it to the
	// principal; when both name the same chain, this record — the
	// transaction's own — says so, and nothing is read from the chain to find
	// out.
	if e, ok := u.entryFor(chain.Account(), chain.Name()); ok {
		return int64(e.Index), nil
	}

	index := c.Height()
	// An anchor chain's appends are also kept as a segment, so the block can
	// build receipts over them from memory. The state before the first
	// append is the chain's head, a live record.
	var seg *merkle.Segment
	if chain.Type() == merkle.ChainTypeAnchor {
		key := SegmentKey(chain.Account(), chain.Name())
		if u.Segments == nil {
			u.Segments = map[string]*merkle.Segment{}
		}
		seg = u.Segments[key]
		if seg == nil {
			head, err := chain.Head().Get()
			if err != nil {
				return 0, errors.UnknownError.WithFormat("load %s chain head: %w", chain.Name(), err)
			}
			seg = &merkle.Segment{First: index, Before: head.Copy(), MarkMask: chain.Inner().MarkMask()}
			u.Segments[key] = seg
		}
	}
	err = c.AddEntry(entry, unique)
	if err != nil {
		return 0, errors.UnknownError.WithFormat("add entry to %s chain: %w", chain.Name(), err)
	}

	// The entry was a duplicate, do not update the ledger
	if index == c.Height() {
		return c.HeightOf(entry)
	}
	if seg != nil {
		seg.Append(entry)
	}

	// Update the ledger
	err = u.DidAddChainEntry(batch, chain.Account(), chain.Name(), chain.Type(), entry, uint64(index), sourceIndex, sourceBlock)
	if err != nil {
		return 0, errors.UnknownError.Wrap(err)
	}

	return index, nil
}
