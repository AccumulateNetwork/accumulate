// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/metrics"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// certifiedDedupRounds is how far back the "already counted" set remembers a
// batch digest. A batch can be re-proposed — a header that never certified is
// requeued and a certificate that never committed has its batches re-proposed
// (header_builder.go, the dedup comment) — so the same batch can reach a
// second certified header, and counting it twice would make this node report
// more certified than accepted, which the harness surfaces as an instrument
// alarm. Beyond this many rounds the batch is long executed and pruned, so a
// duplicate is not possible; the window exists to bound the set, not to
// approximate.
const certifiedDedupRounds = 1000

// certifiedSweepEvery is how often the set is swept for entries older than
// the window.
const certifiedSweepEvery = 128

// countCertifiedOwn counts the transactions of a header of ours that has
// reached a certificate and is in the DAG (#4366, #4369).
//
// Called AFTER the certificate is inserted, not when it is created: an insert
// that fails un-claims the round and requeues the header's batches (A13a,
// vote_handler.go), and counting before the insert would report as certified
// what is about to be proposed again.
//
// Each transaction is counted once, at the first certified header carrying
// its batch. The count comes from the batches the header names — the worker
// that sealed a batch still holds it, in its active store or its retention —
// so nothing new is asked of the worker.
//
// A node in no committee never gets here: validators drop its header before
// voting (vote_handler.go, "Header from unknown validator"), so it collects
// no quorum, creates no certificate, and this counter stays at zero for the
// life of the process. That is the whole point of it.
func (p *Primary) countCertifiedOwn(cert *types.Certificate) {
	if cert == nil || cert.Header == nil {
		return
	}

	round := cert.Round()
	var txns int
	for _, entry := range cert.Header.Payload {
		if !p.claimCertified(entry.Digest, round) {
			continue
		}
		for _, w := range p.workers {
			if w.ID() != entry.Worker {
				continue
			}
			b, err := w.GetBatch(entry.Digest)
			if err == nil && b != nil {
				txns += len(b.Transactions)
			}
			break
		}
	}
	if txns > 0 {
		metrics.CertifiedOwnTransactionsTotal.
			WithLabelValues(strings.ToLower(p.config.Partition)).Add(float64(txns))
	}
}

// claimCertified reports whether this batch is being counted for the first
// time, and records it. It also drops what is too old to be re-proposed.
func (p *Primary) claimCertified(digest types.BatchDigest, round types.Round) bool {
	p.certifiedMu.Lock()
	defer p.certifiedMu.Unlock()

	if p.certifiedBatches == nil {
		p.certifiedBatches = map[types.BatchDigest]types.Round{}
	}
	if _, ok := p.certifiedBatches[digest]; ok {
		return false
	}
	p.certifiedBatches[digest] = round

	if round > p.certifiedHighRound {
		p.certifiedHighRound = round
	}

	// Sweep occasionally, not on every batch: the set is bounded by the
	// window, and an O(n) walk per certified batch is not.
	if p.certifiedHighRound > certifiedDedupRounds && p.certifiedHighRound >= p.certifiedNextSweep {
		cutoff := p.certifiedHighRound - certifiedDedupRounds
		for d, r := range p.certifiedBatches {
			if r < cutoff {
				delete(p.certifiedBatches, d)
			}
		}
		p.certifiedNextSweep = p.certifiedHighRound + certifiedSweepEvery
	}
	return true
}
