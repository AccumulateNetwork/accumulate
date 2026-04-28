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

	// operatorsPage, if non-nil, is the operators key page URL used
	// by OperatorsDeltaAt to extract validator-set updates. Without
	// it, OperatorsDeltaAt returns nil — adequate for steady-state
	// networks where rotation is rare. Production deployments wire
	// this via SetOperatorsPage.
	operatorsPage *url.URL

	// partitionUrl, derived from anchorPool's parent, is the scope
	// for QueryMinorBlock.
	partitionUrl *url.URL

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
//
// The partition URL is derived as the anchor pool's parent.
// OperatorsDeltaAt is dormant until SetOperatorsPage is called.
func NewAPISource(q api.Querier2, anchorPool *url.URL) *APISource {
	var partition *url.URL
	if anchorPool != nil {
		if p, ok := anchorPool.Parent(); ok {
			partition = p
		}
	}
	return &APISource{
		q:            q,
		anchorPool:   anchorPool,
		partitionUrl: partition,
		PageSize:     256,
		cache:        make(map[uint64]*anchorEntry),
	}
}

// SetOperatorsPage installs the operators key page URL. After this
// call, OperatorsDeltaAt(h) returns the operator-keybook updates
// recorded in block h.
func (s *APISource) SetOperatorsPage(u *url.URL) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.operatorsPage = u
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
// transaction at the given height. Pulls from the anchor txn's
// signature chain, decodes messaging.BlockAnchor / SignatureMessage
// wrappers, and surfaces each as a HeaderSignature with both the
// raw key-hash + signature bytes and the protocol.KeySignature for
// protocol-aware verification.
func (s *APISource) Signatures(ctx context.Context, height uint64) ([]HeaderSignature, error) {
	rec, err := s.lookup(ctx, height)
	if err != nil {
		return nil, err
	}
	txnURL := rec.txid.AsUrl()

	var out []HeaderSignature
	var start uint64
	for {
		count := s.PageSize
		expand := true
		page, err := s.q.QuerySignatureChainEntries(ctx, txnURL, &api.ChainQuery{
			Name: "signature",
			Range: &api.RangeOptions{
				Start:  start,
				Count:  &count,
				Expand: &expand,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("query signature chain for height %d: %w", height, err)
		}
		if page == nil || len(page.Records) == 0 {
			break
		}

		for _, e := range page.Records {
			if e == nil || e.Value == nil || e.Value.Message == nil {
				continue
			}
			ks, signable := extractKeySigAndSignable(e.Value.Message)
			if ks == nil {
				continue
			}
			var pkh [32]byte
			copy(pkh[:], ks.GetPublicKeyHash())
			out = append(out, HeaderSignature{
				PublicKeyHash: pkh,
				Signature:     ks.GetSignature(),
				KeySignature:  ks,
				Signable:      signable,
			})
		}
		if uint64(len(page.Records)) < count {
			break
		}
		start += uint64(len(page.Records))
	}
	return out, nil
}

// extractKeySigAndSignable pulls the validator's KeySignature plus
// the Signable it was made over out of the messaging wrappers on the
// anchor txn's signature chain.
//
// For BlockAnchor: signature was made over BlockAnchor.Anchor (the
// SequencedMessage). Returns both.
//
// For SignatureMessage: the Signable is the underlying transaction
// referenced by SignatureMessage.TxID — but we don't have the txn
// inline; the caller would need a separate fetch. Return the
// KeySignature with a nil Signable in that case; verifySig falls
// back to using the Header itself, which only verifies cleanly when
// the Header's Sequenced is set (which it is for BlockAnchor-derived
// headers).
//
// Live anchor signatures on the bootstrap path are BlockAnchor
// wrappers, so the BlockAnchor branch is the hot path.
func extractKeySigAndSignable(m messaging.Message) (protocol.KeySignature, protocol.Signable) {
	switch v := m.(type) {
	case *messaging.BlockAnchor:
		// BlockAnchor.Signature is already typed as KeySignature.
		// BlockAnchor.Anchor is the SequencedMessage validators
		// signed over.
		seq, _ := v.Anchor.(*messaging.SequencedMessage)
		return v.Signature, seq
	case *messaging.SignatureMessage:
		if ks, ok := v.Signature.(protocol.KeySignature); ok {
			return ks, nil
		}
	}
	return nil, nil
}

// OperatorsDeltaAt returns the operators-keybook deltas recorded in
// block `height`. Queries the partition's minor block at that
// height, filters its entries for transactions whose principal is
// the configured operators page, and extracts UpdateKeyPage
// operations.
//
// Returns nil (no deltas, no error) when:
//   - SetOperatorsPage wasn't called (steady-state mode)
//   - The block exists but contains no operators-keybook updates
//
// Returns an error if the underlying queries fail. The walker treats
// an error here as fatal; the launcher exits and the operator
// retries. We deliberately don't paper over query failures —
// returning empty deltas would silently misverify the next block's
// quorum if a rotation got dropped.
func (s *APISource) OperatorsDeltaAt(ctx context.Context, height uint64) ([]OperatorsDelta, error) {
	s.mu.Lock()
	opPage := s.operatorsPage
	partition := s.partitionUrl
	s.mu.Unlock()
	if opPage == nil || partition == nil {
		return nil, nil
	}

	var out []OperatorsDelta
	var entryStart uint64
	for {
		count := s.PageSize
		expand := true
		block, err := s.q.QueryMinorBlock(ctx, partition, &api.BlockQuery{
			Minor: &height,
			EntryRange: &api.RangeOptions{
				Start:  entryStart,
				Count:  &count,
				Expand: &expand,
			},
			OmitEmpty: true,
		})
		if err != nil {
			return nil, fmt.Errorf("query minor block %d: %w", height, err)
		}
		if block == nil || block.Entries == nil || len(block.Entries.Records) == 0 {
			break
		}

		for _, e := range block.Entries.Records {
			if e == nil || e.Account == nil || !e.Account.Equal(opPage) {
				continue
			}
			ops, err := extractKeyPageOps(e.Value)
			if err != nil {
				return nil, fmt.Errorf("block %d entry %d: %w", height, e.Index, err)
			}
			for _, op := range ops {
				delta, err := encodeKeyPageOp(op)
				if err != nil {
					return nil, fmt.Errorf("encode op: %w", err)
				}
				out = append(out, delta)
			}
		}

		if uint64(len(block.Entries.Records)) < count {
			break
		}
		entryStart += uint64(len(block.Entries.Records))
	}
	return out, nil
}

// extractKeyPageOps pulls the Operation slice out of a chain-entry
// value if it's an UpdateKeyPage transaction. Other transaction
// types and non-transaction records return nil.
func extractKeyPageOps(value api.Record) ([]protocol.KeyPageOperation, error) {
	if value == nil {
		return nil, nil
	}
	mr, ok := value.(*api.MessageRecord[messaging.Message])
	if !ok || mr == nil || mr.Message == nil {
		return nil, nil
	}
	tm, ok := mr.Message.(*messaging.TransactionMessage)
	if !ok || tm.Transaction == nil {
		return nil, nil
	}
	body, ok := tm.Transaction.Body.(*protocol.UpdateKeyPage)
	if !ok {
		return nil, nil
	}
	return body.Operation, nil
}

// encodeKeyPageOp marshals a KeyPageOperation into the
// OperatorsDelta wire format. The keybookat package owns the
// canonical encoding, but we reproduce its byte-level contract here
// to avoid pulling keybookat as an import (preserves the layering
// where headerwalk doesn't depend on consumer packages).
func encodeKeyPageOp(op protocol.KeyPageOperation) (OperatorsDelta, error) {
	data, err := op.MarshalBinary()
	if err != nil {
		return OperatorsDelta{}, fmt.Errorf("marshal %T: %w", op, err)
	}
	return OperatorsDelta{
		Kind:    op.Type().String(),
		Payload: data,
	}, nil
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

