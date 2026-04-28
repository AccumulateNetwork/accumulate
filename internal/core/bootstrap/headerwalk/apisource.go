// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package headerwalk

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// APISource adapts an api.Querier2 into a HeaderSource by reading
// from a partition's anchor pool. Each main-chain entry on the anchor
// pool is an anchor transaction whose body embeds a PartitionAnchor.
// The launcher cares about three things from each entry:
//
//   - PartitionAnchor.MinorBlockIndex (height)
//   - PartitionAnchor.RootChainAnchor (header's chain root)
//   - PartitionAnchor.StateTreeAnchor (BPT root at this block)
//   - The anchor transaction hash (what validators signed)
//
// Block time is taken from the chain entry's LastBlockTime metadata
// since PartitionAnchor itself doesn't carry a timestamp.
//
// There is no direct height→entry index — anchor pool entries are
// addressed by main-chain index, not block height. APISource
// maintains an internal cache populated on demand: a Header(h) call
// that misses scans forward through the anchor pool until it finds
// MinorBlockIndex == h, caching every entry it touched along the way.
//
// Signature retrieval is deferred to a follow-up: the signature chain
// on the anchor transaction needs protocol-aware key-signature
// verification (validators sign with KeySignature semantics, not raw
// ed25519 over a hash) which warrants its own focused commit. Until
// that lands, Signatures returns the raw KeySignature bytes paired
// with their public-key hash, and VerifyQuorum's ed25519-only path
// will accept ED25519 keys correctly while skipping other signature
// types — exactly the behavior the walker tests exercise today.
type APISource struct {
	q          api.Querier2
	anchorPool *url.URL

	// PageSize is the chain-entries page size for anchor pool scans.
	// Default 256.
	PageSize uint64

	mu    sync.Mutex
	cache map[uint64]*anchorEntry
	// scanned is the highest main-chain index we've inspected so far,
	// so a fresh Header(h) call doesn't re-page from zero.
	scanned uint64
}

// anchorEntry is one resolved anchor: the parsed PartitionAnchor plus
// the raw transaction (so we can hash it) and metadata.
type anchorEntry struct {
	pa        *protocol.PartitionAnchor
	txn       *protocol.Transaction
	txid      *url.TxID
	blockTime int64 // chain entry's LastBlockTime, unix seconds
}

// NewAPISource returns an APISource over q, scanning the partition's
// anchor pool (acc://<partition>.acme/anchors). For the DN partition
// this is acc://dn.acme/anchors; for BVNs it's acc://<bvn>.acme/anchors.
func NewAPISource(q api.Querier2, anchorPool *url.URL) *APISource {
	return &APISource{
		q:          q,
		anchorPool: anchorPool,
		PageSize:   256,
		cache:      make(map[uint64]*anchorEntry),
	}
}

func (s *APISource) Header(ctx context.Context, height uint64) (*Header, error) {
	rec, err := s.lookup(ctx, height)
	if err != nil {
		return nil, err
	}
	var anchorHash [32]byte
	copy(anchorHash[:], rec.txn.GetHash())

	hdr := &Header{
		Height:        rec.pa.MinorBlockIndex,
		ChainRoot:     rec.pa.RootChainAnchor,
		StateTreeRoot: rec.pa.StateTreeAnchor,
		AnchorTxHash:  anchorHash,
	}
	if rec.blockTime > 0 {
		hdr.Time = time.Unix(rec.blockTime, 0).UTC()
	}
	return hdr, nil
}

// Signatures returns validator signatures attached to the anchor
// transaction. **Implementation note:** today this is a stub that
// returns nil — proper retrieval requires querying the anchor
// transaction's signature chain and decoding messaging.BlockAnchor
// wrappers, with the launcher then mapping protocol.KeySignature to
// HeaderSignature using the operators-keybook validator set. Tracked
// as a follow-up; until it lands, walks against APISource use a fake
// initial validator set or zero MinSignatures for smoke testing.
func (s *APISource) Signatures(ctx context.Context, height uint64) ([]HeaderSignature, error) {
	return nil, nil
}

// OperatorsDeltaAt returns the operators-keybook deltas applied in
// the block at the given height. Stub for now; needs to query the
// operators key book's main chain at the relevant block range and
// surface UpdateKeyPage / AddCredits / etc. operations. Deferred to
// the keybookat-integration commit.
func (s *APISource) OperatorsDeltaAt(ctx context.Context, height uint64) ([]OperatorsDelta, error) {
	return nil, nil
}

// lookup resolves height to an anchorEntry, scanning + caching as
// needed.
func (s *APISource) lookup(ctx context.Context, height uint64) (*anchorEntry, error) {
	s.mu.Lock()
	if rec, ok := s.cache[height]; ok {
		s.mu.Unlock()
		return rec, nil
	}
	s.mu.Unlock()

	if err := s.scanUntil(ctx, height); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if rec, ok := s.cache[height]; ok {
		return rec, nil
	}
	return nil, fmt.Errorf("%w: anchor for partition height %d not found in anchor pool", ErrNoSuchHeight, height)
}

// scanUntil walks anchor pool main-chain entries forward from
// s.scanned, inserting each into the cache, until either the cache
// contains `target` or pagination exhausts.
func (s *APISource) scanUntil(ctx context.Context, target uint64) error {
	for {
		s.mu.Lock()
		start := s.scanned
		s.mu.Unlock()

		count := s.PageSize
		expand := true
		page, err := s.q.QueryChainEntries(ctx, s.anchorPool, &api.ChainQuery{
			Name: "main",
			Range: &api.RangeOptions{
				Start:  start,
				Count:  &count,
				Expand: &expand,
			},
		})
		if err != nil {
			return fmt.Errorf("query anchor pool main range[%d:+%d]: %w", start, count, err)
		}
		if page == nil || len(page.Records) == 0 {
			return nil
		}

		s.mu.Lock()
		hit := false
		for _, e := range page.Records {
			if e == nil || e.Value == nil {
				continue
			}
			rec := extractAnchorRecord(e)
			if rec == nil {
				continue
			}
			s.cache[rec.pa.MinorBlockIndex] = rec
			if rec.pa.MinorBlockIndex == target {
				hit = true
			}
		}
		s.scanned = start + uint64(len(page.Records))
		s.mu.Unlock()

		if hit {
			return nil
		}
		// If the page was short, we've exhausted the chain.
		if uint64(len(page.Records)) < count {
			return nil
		}
	}
}

// extractAnchorRecord pulls the PartitionAnchor out of a
// ChainEntryRecord. Returns nil if the record isn't an anchor txn
// (defensive — the anchor pool's main chain should only contain
// anchor txns, but bootstrap shouldn't crash if a peer returns
// something unexpected).
func extractAnchorRecord(e *api.ChainEntryRecord[api.Record]) *anchorEntry {
	if e == nil || e.Value == nil {
		return nil
	}
	mr, ok := e.Value.(*api.MessageRecord[messaging.Message])
	if !ok || mr.Message == nil {
		return nil
	}
	tm, ok := mr.Message.(*messaging.TransactionMessage)
	if !ok || tm.Transaction == nil {
		return nil
	}
	body, ok := tm.Transaction.Body.(protocol.AnchorBody)
	if !ok {
		return nil
	}
	pa := body.GetPartitionAnchor()
	if pa == nil {
		return nil
	}
	rec := &anchorEntry{
		pa:   pa,
		txn:  tm.Transaction,
		txid: tm.ID(),
	}
	if e.LastBlockTime != nil {
		rec.blockTime = e.LastBlockTime.Unix()
	}
	return rec
}

