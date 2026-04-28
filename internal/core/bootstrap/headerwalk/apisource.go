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

// APISource is the v2-corrected HeaderSource over an api.Querier2.
//
// Trust phase model (see docs/plans/bootstrap-v2.md):
//
//   - The launcher targets a (DN, BVN) pair. Bootstrapping the BVN
//     hangs on first proving the DN's major-block spine; once
//     proven, BVN→DN anchors stored in trusted DN state give the
//     BVN's StateTreeAnchor for free.
//   - DN's outgoing DAs land on each receiving BVN's anchor pool.
//     Their validator-quorum signatures (one BlockAnchor message per
//     DN validator) attach to the DA txn's signature chain on the
//     BVN side. DN itself does NOT store DN-validator signatures on
//     its own outgoing DAs.
//   - So this source is constructed against the *chosen BVN's*
//     anchor pool, not the DN's. It walks the BVN's MajorBlockChain
//     for skip-pagination, fetches each major-block-boundary DA
//     from the BVN's main chain, and pulls signatures off the DA
//     txn's signature chain on the BVN side.
//
// The validator set used to verify signatures is the *DN's*
// operators key page — `dn.acme/operators/1`. SetOperatorsPage
// records this URL so OperatorsDeltaAt can query it for rotation
// updates during the back-walk.
//
// Today (no operators rotations have occurred on mainnet)
// OperatorsDeltaAt returns nil for every major block — the
// steady-state hot path. The keybookat machinery on the consumer
// side handles non-empty deltas correctly when they appear; this
// source's wiring of "walk DN operators main chain for UpdateKeyPage
// in the right minor-block range" is deferred to a sub-phase that
// activates when mainnet first rotates.
type APISource struct {
	q             api.Querier2
	bvnAnchorPool *url.URL // chosen BVN's acc://<bvn>.acme/anchors

	// dnOperatorsPage is dn.acme/operators/1, set via
	// SetOperatorsPage. Used by OperatorsDeltaAt; nil disables
	// rotation tracking.
	dnOperatorsPage *url.URL

	// PageSize is the chain-entries page size for index/main-chain
	// scans. Default 256.
	PageSize uint64

	mu    sync.Mutex
	cache map[uint64]*majorBlockEntry // keyed by DN major-block index
}

// majorBlockEntry is one resolved major-block boundary on the BVN
// side. Held in the cache so Header()/Signatures()/OperatorsDeltaAt
// for the same majorIdx don't re-query.
type majorBlockEntry struct {
	majorIdx uint64

	// daTxID is the txid of the DA from DN that landed on the BVN's
	// anchor pool at this major-block boundary.
	daTxID *url.TxID

	// pa is the DN's PartitionAnchor extracted from the DA's body.
	// pa.MajorBlockIndex equals majorIdx. pa.StateTreeAnchor is
	// DN's BPT root at this boundary.
	pa *protocol.PartitionAnchor

	// sequenced is the SequencedMessage validators signed over —
	// extracted from the BlockAnchor wrapper's Anchor field on the
	// signature chain.
	sequenced *messaging.SequencedMessage

	// blockTime is unix seconds; from the BVN MajorBlockChain
	// IndexEntry's BlockTime.
	blockTime int64
}

// NewAPISource returns an APISource over q targeting the chosen
// BVN's anchor pool URL. Use SetOperatorsPage to enable
// OperatorsDeltaAt.
func NewAPISource(q api.Querier2, bvnAnchorPool *url.URL) *APISource {
	return &APISource{
		q:             q,
		bvnAnchorPool: bvnAnchorPool,
		PageSize:      256,
		cache:         make(map[uint64]*majorBlockEntry),
	}
}

// SetOperatorsPage installs the DN operators key page URL
// (`dn.acme/operators/1`). After this call, OperatorsDeltaAt(N)
// returns the operator-keybook deltas in DN's minor blocks between
// major-block N-1 and N. Without it, OperatorsDeltaAt is dormant.
func (s *APISource) SetOperatorsPage(u *url.URL) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dnOperatorsPage = u
}

// Header returns the trust-phase Header for DN major-block
// majorIdx. Caches the result so Signatures() and
// OperatorsDeltaAt() can reuse the same lookup.
func (s *APISource) Header(ctx context.Context, majorIdx uint64) (*Header, error) {
	rec, err := s.lookup(ctx, majorIdx)
	if err != nil {
		return nil, err
	}
	hdr := &Header{
		Height:        rec.pa.MajorBlockIndex,
		ChainRoot:     rec.pa.RootChainAnchor,
		StateTreeRoot: rec.pa.StateTreeAnchor,
		Sequenced:     rec.sequenced,
	}
	if rec.blockTime > 0 {
		hdr.Time = time.Unix(rec.blockTime, 0).UTC()
	}
	return hdr, nil
}

// Signatures returns DN-validator signatures attached to the major-
// block-boundary DA at majorIdx. Each is a HeaderSignature carrying
// both the raw bytes and the protocol.KeySignature, with Signable
// pointing at the SequencedMessage that was signed.
func (s *APISource) Signatures(ctx context.Context, majorIdx uint64) ([]HeaderSignature, error) {
	rec, err := s.lookup(ctx, majorIdx)
	if err != nil {
		return nil, err
	}
	if rec.daTxID == nil {
		return nil, nil
	}

	txnURL := rec.daTxID.AsUrl()
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
			return nil, fmt.Errorf("query signature chain for major-block %d: %w", majorIdx, err)
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

// OperatorsDeltaAt returns DN operators-keybook updates that landed
// in DN's minor blocks aggregated under major-block majorIdx.
// Today returns nil — wiring DN's MajorBlockChain → minor-block
// range → operators-page UpdateKeyPage scan is deferred to a sub-
// phase that activates when mainnet first rotates operators.
//
// keybookat.ApplyDelta on the consumer side handles non-empty
// deltas correctly when they appear, so this stub is safe for
// the no-rotation hot path.
func (s *APISource) OperatorsDeltaAt(ctx context.Context, majorIdx uint64) ([]OperatorsDelta, error) {
	s.mu.Lock()
	page := s.dnOperatorsPage
	s.mu.Unlock()
	if page == nil {
		return nil, nil
	}
	// TODO: query DN's MajorBlockChain to find the minor-block
	// range for major-block majorIdx, then scan dnOperatorsPage's
	// main chain for UpdateKeyPage txns in that range. For now,
	// returning nil matches the steady-state path.
	return nil, nil
}

// --- internals ----------------------------------------------------

// lookup resolves a major-block index to its BVN-side DA + the
// SequencedMessage validators signed over.
func (s *APISource) lookup(ctx context.Context, majorIdx uint64) (*majorBlockEntry, error) {
	s.mu.Lock()
	if rec, ok := s.cache[majorIdx]; ok {
		s.mu.Unlock()
		return rec, nil
	}
	s.mu.Unlock()

	rec, err := s.fetchMajorBlock(ctx, majorIdx)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.cache[majorIdx] = rec
	s.mu.Unlock()
	return rec, nil
}

// fetchMajorBlock pulls the IndexEntry for major-block majorIdx from
// the BVN's MajorBlockChain, follows its Source to the BVN's main
// chain, fetches the DA txn, and pulls one BlockAnchor message off
// the signature chain to extract the SequencedMessage.
func (s *APISource) fetchMajorBlock(ctx context.Context, majorIdx uint64) (*majorBlockEntry, error) {
	if majorIdx == 0 {
		return nil, fmt.Errorf("%w: major-block 0 doesn't exist (genesis is major-block 1)", ErrNoSuchHeight)
	}
	pos := majorIdx - 1 // major-block 1 → MajorBlockChain entry 0

	// 1. BVN's MajorBlockChain entry at position (majorIdx-1) →
	// IndexEntry pointing at the BVN main-chain position of the
	// boundary DA.
	one := uint64(1)
	idxPage, err := s.q.QueryIndexChainEntries(ctx, s.bvnAnchorPool, &api.ChainQuery{
		Name: "major-block",
		Range: &api.RangeOptions{
			Start: pos,
			Count: &one,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("query bvn major-block index entry %d: %w", pos, err)
	}
	if idxPage == nil || len(idxPage.Records) == 0 {
		return nil, fmt.Errorf("%w: bvn major-block index entry %d not found", ErrNoSuchHeight, pos)
	}
	indexRec := idxPage.Records[0]
	if indexRec == nil || indexRec.Value == nil || indexRec.Value.Value == nil {
		return nil, fmt.Errorf("bvn major-block entry %d empty", pos)
	}
	indexEntry := indexRec.Value.Value
	mainChainPos := indexEntry.Source

	// 2. BVN's main chain entry at mainChainPos → the DA txn.
	mainPage, err := s.q.QueryChainEntries(ctx, s.bvnAnchorPool, &api.ChainQuery{
		Name: "main",
		Range: &api.RangeOptions{
			Start:  mainChainPos,
			Count:  &one,
			Expand: ptrBool(true),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("query bvn main chain entry %d: %w", mainChainPos, err)
	}
	if mainPage == nil || len(mainPage.Records) == 0 {
		return nil, fmt.Errorf("bvn main chain entry %d not found", mainChainPos)
	}
	mainRec := mainPage.Records[0]
	pa, txn, txid, err := extractDAFromMainEntry(mainRec)
	if err != nil {
		return nil, fmt.Errorf("extract DA at major-block %d: %w", majorIdx, err)
	}

	// 3. Pull one BlockAnchor message off the DA's signature chain
	// to extract the SequencedMessage. Any of them works — they all
	// wrap the same seq.
	seq, err := s.fetchSequencedFromSignatures(ctx, txid, txn)
	if err != nil {
		return nil, err
	}

	// Block time from the IndexEntry.
	var blockTime int64
	if indexEntry.BlockTime != nil {
		blockTime = indexEntry.BlockTime.Unix()
	}

	return &majorBlockEntry{
		majorIdx:  majorIdx,
		daTxID:    txid,
		pa:        pa,
		sequenced: seq,
		blockTime: blockTime,
	}, nil
}

// fetchSequencedFromSignatures pulls the first available BlockAnchor
// off the DA txn's signature chain and returns its SequencedMessage.
// Falls back to constructing the seq locally from the txn if the
// signature chain is empty (shouldn't happen on a well-formed
// network, but defensive).
func (s *APISource) fetchSequencedFromSignatures(ctx context.Context, txid *url.TxID, txn *protocol.Transaction) (*messaging.SequencedMessage, error) {
	one := uint64(1)
	expand := true
	page, err := s.q.QuerySignatureChainEntries(ctx, txid.AsUrl(), &api.ChainQuery{
		Name: "signature",
		Range: &api.RangeOptions{
			Start:  0,
			Count:  &one,
			Expand: &expand,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("query signature chain for sequenced: %w", err)
	}
	if page == nil || len(page.Records) == 0 {
		return nil, fmt.Errorf("DA txn %x has no signatures — peer may not have received the BlockAnchor messages", txid.Hash())
	}
	for _, e := range page.Records {
		if e == nil || e.Value == nil || e.Value.Message == nil {
			continue
		}
		if ba, ok := e.Value.Message.(*messaging.BlockAnchor); ok {
			if seq, ok := ba.Anchor.(*messaging.SequencedMessage); ok {
				return seq, nil
			}
		}
	}
	return nil, fmt.Errorf("DA txn %x signature chain has no BlockAnchor wrapper", txid.Hash())
}

// extractDAFromMainEntry casts a BVN main-chain entry to its
// underlying DA transaction. Returns the PartitionAnchor (DN's
// PartitionAnchor embedded in the DA body), the raw transaction,
// and the txid.
func extractDAFromMainEntry(e *api.ChainEntryRecord[api.Record]) (*protocol.PartitionAnchor, *protocol.Transaction, *url.TxID, error) {
	if e == nil || e.Value == nil {
		return nil, nil, nil, fmt.Errorf("empty main-chain entry")
	}
	mr, ok := e.Value.(*api.MessageRecord[messaging.Message])
	if !ok || mr.Message == nil {
		return nil, nil, nil, fmt.Errorf("main-chain entry value is not a MessageRecord")
	}
	tm, ok := mr.Message.(*messaging.TransactionMessage)
	if !ok || tm.Transaction == nil {
		return nil, nil, nil, fmt.Errorf("main-chain entry message is not a TransactionMessage")
	}
	body, ok := tm.Transaction.Body.(protocol.AnchorBody)
	if !ok {
		return nil, nil, nil, fmt.Errorf("main-chain entry body is %T, not an AnchorBody", tm.Transaction.Body)
	}
	pa := body.GetPartitionAnchor()
	if pa == nil {
		return nil, nil, nil, fmt.Errorf("anchor body has nil PartitionAnchor")
	}
	return pa, tm.Transaction, tm.ID(), nil
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
func extractKeySigAndSignable(m messaging.Message) (protocol.KeySignature, protocol.Signable) {
	switch v := m.(type) {
	case *messaging.BlockAnchor:
		seq, _ := v.Anchor.(*messaging.SequencedMessage)
		return v.Signature, seq
	case *messaging.SignatureMessage:
		if ks, ok := v.Signature.(protocol.KeySignature); ok {
			return ks, nil
		}
	}
	return nil, nil
}

func ptrBool(b bool) *bool { return &b }
