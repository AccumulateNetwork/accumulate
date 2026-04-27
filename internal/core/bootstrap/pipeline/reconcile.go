// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pipeline

import (
	"context"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// ReconcileResult reports the outcome of BPT reconciliation against
// the peer.
type ReconcileResult struct {
	// LeavesVerified is the count of pulled accounts whose stored state
	// hash matches the peer's BPT leaf and whose Merkle proof validates.
	LeavesVerified int

	// PeerBptRoot is the most recent BPT root reported by the peer
	// during reconciliation. The trust-bundle path (#3983) will compare
	// this to the validator-quorum-signed StateTreeAnchor to close the
	// loop on trustless verification.
	PeerBptRoot [32]byte

	// RootsDiverged reports whether peer-reported BptRoots changed
	// between leaf queries. The peer can advance between queries on a
	// live network so this is informational, not an error.
	RootsDiverged bool
}

// reconcileBPT validates that each pulled account's locally stored state
// is consistent with the peer's BPT (issue #3980).
//
// Per-account hard checks:
//
//  1. Peer returns a BPT leaf and a Merkle proof.
//  2. Proof.Validate() succeeds (proof is cryptographically sound).
//  3. Proof.Start matches the leaf's ValueHash.
//  4. Proof.Anchor matches the leaf's reported BptRoot.
//  5. Local Account.Hash() matches the peer's ValueHash. This is the
//     core "we stored what the network has" guarantee.
//
// Cross-account soft check: peer-reported BptRoots are recorded; a
// change between queries is logged but not fatal (the network advances
// during the bootstrap pull window).
//
// Tightening the cross-account check to a single height requires a
// historical-BPT query method (currently GetBptLeaf returns the leaf
// at the current root only). When that lands, this can fail closed on
// divergence.
func reconcileBPT(
	ctx context.Context,
	q api.Querier2,
	batch *database.Batch,
	scope *url.URL,
	accounts []*url.URL,
	logf func(string, ...any),
) (*ReconcileResult, error) {
	res := &ReconcileResult{}
	var firstRoot [32]byte
	first := true

	for _, u := range accounts {
		keyHash := record.NewKey("Account", u).Hash()

		rec, err := q.QueryBptLeaf(ctx, scope, &api.BptLeafQuery{Key: keyHash})
		if err != nil {
			return nil, fmt.Errorf("query BPT leaf for %s: %w", u, err)
		}
		if rec == nil || rec.Proof == nil {
			return nil, fmt.Errorf("%s: empty BPT leaf response", u)
		}

		if !rec.Proof.Validate(nil) {
			return nil, fmt.Errorf("%s: BPT proof failed to validate", u)
		}

		if len(rec.Proof.Start) != 32 {
			return nil, fmt.Errorf("%s: proof start wrong length (%d)", u, len(rec.Proof.Start))
		}
		var start [32]byte
		copy(start[:], rec.Proof.Start)
		if start != rec.ValueHash {
			return nil, fmt.Errorf("%s: proof start %x != reported value %x", u, start[:8], rec.ValueHash[:8])
		}

		if len(rec.Proof.Anchor) != 32 {
			return nil, fmt.Errorf("%s: proof anchor wrong length (%d)", u, len(rec.Proof.Anchor))
		}
		var anchor [32]byte
		copy(anchor[:], rec.Proof.Anchor)
		if anchor != rec.BptRoot {
			return nil, fmt.Errorf("%s: proof anchor %x != reported root %x", u, anchor[:8], rec.BptRoot[:8])
		}

		localHash, err := batch.Account(u).Hash()
		if err != nil {
			return nil, fmt.Errorf("%s: local state hash: %w", u, err)
		}
		if localHash != rec.ValueHash {
			return nil, fmt.Errorf("%s: local state hash %x != peer leaf value %x — pulled state diverges from network", u, localHash[:8], rec.ValueHash[:8])
		}

		res.LeavesVerified++
		res.PeerBptRoot = rec.BptRoot
		if first {
			firstRoot = rec.BptRoot
			first = false
		} else if rec.BptRoot != firstRoot {
			res.RootsDiverged = true
		}
	}

	if res.RootsDiverged {
		logf("    note: peer BptRoot advanced during reconciliation (first=%x last=%x); per-leaf checks still hold",
			firstRoot[:8], res.PeerBptRoot[:8])
	}
	return res, nil
}
