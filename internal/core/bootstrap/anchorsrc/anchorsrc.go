// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package anchorsrc is the production AnchorSource for bootstrap-v3.
//
// It walks a peer's anchor pool main chain looking for the most
// recent signed anchor that carries the target partition's
// StateTreeAnchor, then verifies the validator-quorum signatures
// against the local validator keypage (pulled in spine phase).
//
// Trust model:
//
//   - The anchor body comes from a peer that, by construction in the
//     orchestrator's flow, advertises ACTIVE/COMPLETE. We do not
//     trust the peer; we verify the anchor's signatures against
//     OUR copy of the validator keypage.
//   - The keypage at <partition>.acme/operators/1 is pulled in
//     ModeFullSpine during phase 1 of the orchestrator. AnchorSource
//     reads it from the local DB, not from the peer.
//   - An anchor is accepted iff valid signatures from distinct
//     keypage entries reach AcceptThreshold.
//
// Cross-partition routing:
//
//   - To verify partition P's BPT root, AnchorSource looks at anchors
//     PRODUCED BY P. Those live on a different partition's anchor
//     pool — the receiving partition. So:
//     * To verify DN's BPT root: query a BVN's acme/anchors for
//       DirectoryAnchors.
//     * To verify a BVN's BPT root: query dn.acme/anchors for
//       BlockValidatorAnchors from that BVN.
//   - Operator passes peerAnchorPool — the URL of the receiving
//     partition's anchor pool that holds anchors from P.
//
// Round-trip cost (typical): one chain query for the most recent N
// main-chain entries, one transaction query per candidate, one
// keypage read from local DB. Verification cost is N elliptic-curve
// verifies (N ≈ validator count).
package anchorsrc

import (
	"bytes"
	"context"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Source implements orchestrator.AnchorSource against a peer's
// v3 query API plus a local DB for validator-set lookup.
type Source struct {
	q              api.Querier2
	peerAnchorPool *url.URL
	localDB        *database.Database

	// scanWindow is how many of the most recent main-chain entries
	// we scan when looking for a fresh anchor. Walking back further
	// is wasted work — the orchestrator polls on a ticker, so a
	// missed anchor is picked up on the next tick.
	scanWindow uint64
}

// New constructs a Source.
//
//   - q: any v3 Querier (jsonrpc/websocket/p2p Client all work).
//   - peerAnchorPool: the receiving partition's anchor pool URL
//     (e.g. dn.acme/anchors when verifying a BVN; bvn-X.acme/anchors
//     when verifying the DN).
//   - localDB: the launcher's local DB. AnchorSource reads
//     <syncing-partition>.acme/operators/1 from here.
func New(q api.Querier, peerAnchorPool *url.URL, localDB *database.Database) (*Source, error) {
	if q == nil {
		return nil, fmt.Errorf("anchorsrc.New: querier required")
	}
	if peerAnchorPool == nil {
		return nil, fmt.Errorf("anchorsrc.New: peerAnchorPool required")
	}
	if localDB == nil {
		return nil, fmt.Errorf("anchorsrc.New: localDB required")
	}
	return &Source{
		q:              api.Querier2{Querier: q},
		peerAnchorPool: peerAnchorPool,
		localDB:        localDB,
		scanWindow:     16,
	}, nil
}

// LatestAnchor implements orchestrator.AnchorSource.
//
// Returns (block, anchor, nil) when a verified signed anchor is
// found, (0, zero, nil) when none is available yet (the orchestrator
// retries), and (_, _, err) on transport or DB failures.
func (s *Source) LatestAnchor(ctx context.Context, partition string) (uint64, [32]byte, error) {
	// PartitionAnchor.Source is the partition that produced the
	// anchor, e.g. acc://dn.acme — NOT the anchor pool URL.
	wantSource := protocol.PartitionUrl(partition)

	// Walk the latest main-chain entries on the peer's anchor pool.
	// QueryMainChainEntries returns entries with their transaction
	// messages already populated.
	count := s.scanWindow
	expand := true
	page, err := s.q.QueryMainChainEntries(ctx, s.peerAnchorPool, &api.ChainQuery{
		Name: "main",
		Range: &api.RangeOptions{
			FromEnd: true,
			Count:   &count,
			Expand:  &expand,
		},
	})
	if err != nil {
		return 0, [32]byte{}, fmt.Errorf("query main chain: %w", err)
	}
	if page == nil || len(page.Records) == 0 {
		return 0, [32]byte{}, nil
	}

	// Walk newest → oldest. First valid anchor wins.
	for i := len(page.Records) - 1; i >= 0; i-- {
		entry := page.Records[i]
		if entry == nil || entry.Value == nil || entry.Value.Message == nil {
			continue
		}
		txnMsg := entry.Value.Message
		if txnMsg.Transaction == nil {
			continue
		}
		anchor, ok := txnMsg.Transaction.Body.(protocol.AnchorBody)
		if !ok {
			continue
		}
		pa := anchor.GetPartitionAnchor()
		if pa == nil || pa.Source == nil {
			continue
		}
		if !pa.Source.Equal(wantSource) {
			continue
		}
		// Skip anchors without a major-block boundary — the launcher
		// converges against major-block anchors only.
		if pa.MajorBlockIndex == 0 {
			continue
		}
		// Candidate matches. Verify its signatures.
		if err := s.verifyQuorum(ctx, partition, entry.Value); err != nil {
			// Don't bubble — try the next candidate. Real failures
			// (transport, DB) bubble. Below-quorum is "skip and try
			// next."
			continue
		}
		return pa.MajorBlockIndex, pa.StateTreeAnchor, nil
	}

	return 0, [32]byte{}, nil
}

// verifyQuorum returns nil iff the validator quorum signed this
// anchor transaction. Loads the partition's validator keypage from
// the local DB (populated in spine phase).
func (s *Source) verifyQuorum(_ context.Context, partition string, txn *api.MessageRecord[*messaging.TransactionMessage]) error {
	keypage, err := s.loadValidatorKeypage(partition)
	if err != nil {
		return fmt.Errorf("load validator keypage: %w", err)
	}
	if keypage.AcceptThreshold == 0 {
		return fmt.Errorf("validator keypage has zero accept threshold")
	}

	if txn.Signatures == nil || len(txn.Signatures.Records) == 0 {
		return fmt.Errorf("anchor has no signatures")
	}

	// Each SignatureSetRecord groups signatures from one signer
	// account. Walk every signature and count distinct keypage
	// entries that signed validly.
	signed := make(map[int]bool)
	for _, set := range txn.Signatures.Records {
		if set == nil || set.Signatures == nil {
			continue
		}
		for _, sigMsg := range set.Signatures.Records {
			if sigMsg == nil || sigMsg.Message == nil {
				continue
			}
			sm, ok := sigMsg.Message.(*messaging.SignatureMessage)
			if !ok {
				continue
			}
			keySig, ok := sm.Signature.(protocol.KeySignature)
			if !ok {
				continue
			}
			idx := keypageIndexOf(keypage, keySig)
			if idx < 0 {
				continue
			}
			// The transaction hash is what's actually signed.
			if !verifySig(keySig, txn.Message.Transaction) {
				continue
			}
			signed[idx] = true
		}
	}

	if uint64(len(signed)) < keypage.AcceptThreshold {
		return fmt.Errorf("only %d/%d validators signed", len(signed), keypage.AcceptThreshold)
	}
	return nil
}

// loadValidatorKeypage reads <partition>.acme/operators/1 from the
// local DB. The page is populated in the orchestrator's spine phase.
func (s *Source) loadValidatorKeypage(partition string) (*protocol.KeyPage, error) {
	pageURL := protocol.PartitionUrl(partition).JoinPath(protocol.Operators, "1")
	batch := s.localDB.Begin(false)
	defer batch.Discard()

	var acct protocol.Account
	if err := batch.Account(pageURL).Main().GetAs(&acct); err != nil {
		return nil, fmt.Errorf("read %s: %w", pageURL, err)
	}
	page, ok := acct.(*protocol.KeyPage)
	if !ok {
		return nil, fmt.Errorf("%s is not a KeyPage (got %T)", pageURL, acct)
	}
	return page, nil
}

// keypageIndexOf returns the 0-based index of the key in the page
// whose PublicKeyHash matches the signature's PublicKeyHash, or -1
// if not found. KeySpec stores the public-key hash; KeySignature
// computes it from the public key it carries.
func keypageIndexOf(page *protocol.KeyPage, sig protocol.KeySignature) int {
	want := sig.GetPublicKeyHash()
	for i, k := range page.Keys {
		if k != nil && bytes.Equal(k.PublicKeyHash, want) {
			return i
		}
	}
	return -1
}

// verifySig wraps KeySignature.Verify with the transaction envelope
// as the signed payload. KeySignature embeds UserSignature which
// provides Verify(Signature, Signable).
func verifySig(sig protocol.KeySignature, txn *protocol.Transaction) bool {
	return sig.Verify(sig, txn)
}
