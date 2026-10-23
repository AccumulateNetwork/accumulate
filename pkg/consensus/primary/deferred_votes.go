// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"log/slog"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// deferredVotes records, per header whose vote is deferred, the parent
// certificates and batches it lacks, so the vote is re-tested the moment the
// last of them lands rather than on the author's next rebroadcast (#4611;
// consensus.md invariant 11). The rebroadcast stays the backstop: a
// notification missed here only costs the second it used to.
type deferredVotes struct {
	mu       sync.Mutex
	headers  map[types.HeaderDigest]*deferredVote
	byBatch  map[types.BatchDigest][]types.HeaderDigest
	byParent map[types.CertificateDigest][]types.HeaderDigest
}

type deferredVote struct {
	header  *types.Header
	batches map[types.BatchDigest]struct{}
	parents map[types.CertificateDigest]struct{}
}

func newDeferredVotes() *deferredVotes {
	return &deferredVotes{
		headers:  map[types.HeaderDigest]*deferredVote{},
		byBatch:  map[types.BatchDigest][]types.HeaderDigest{},
		byParent: map[types.CertificateDigest][]types.HeaderDigest{},
	}
}

// wait records what header lacks, replacing anything recorded for it before.
func (dv *deferredVotes) wait(header *types.Header, parents []types.CertificateDigest, batches []types.BatchDigest) {
	digest := header.Digest()
	w := &deferredVote{
		header:  header,
		batches: make(map[types.BatchDigest]struct{}, len(batches)),
		parents: make(map[types.CertificateDigest]struct{}, len(parents)),
	}
	dv.mu.Lock()
	defer dv.mu.Unlock()
	for _, d := range batches {
		w.batches[d] = struct{}{}
		dv.byBatch[d] = append(dv.byBatch[d], digest)
	}
	for _, d := range parents {
		w.parents[d] = struct{}{}
		dv.byParent[d] = append(dv.byParent[d], digest)
	}
	dv.headers[digest] = w
}

// batchArrived returns the headers that lacked only this batch.
func (dv *deferredVotes) batchArrived(d types.BatchDigest) []*types.Header {
	dv.mu.Lock()
	defer dv.mu.Unlock()
	waiting, ok := dv.byBatch[d]
	if !ok {
		return nil
	}
	delete(dv.byBatch, d)
	var ready []*types.Header
	for _, hd := range waiting {
		if w, ok := dv.headers[hd]; ok {
			if _, lacked := w.batches[d]; lacked {
				delete(w.batches, d)
				ready = dv.takeIfComplete(hd, w, ready)
			}
		}
	}
	return ready
}

// parentArrived returns the headers that lacked only this parent.
func (dv *deferredVotes) parentArrived(d types.CertificateDigest) []*types.Header {
	dv.mu.Lock()
	defer dv.mu.Unlock()
	waiting, ok := dv.byParent[d]
	if !ok {
		return nil
	}
	delete(dv.byParent, d)
	var ready []*types.Header
	for _, hd := range waiting {
		if w, ok := dv.headers[hd]; ok {
			if _, lacked := w.parents[d]; lacked {
				delete(w.parents, d)
				ready = dv.takeIfComplete(hd, w, ready)
			}
		}
	}
	return ready
}

// takeIfComplete removes a header that lacks nothing more and appends it to
// ready. The caller must hold mu.
func (dv *deferredVotes) takeIfComplete(hd types.HeaderDigest, w *deferredVote, ready []*types.Header) []*types.Header {
	if len(w.batches) > 0 || len(w.parents) > 0 {
		return ready
	}
	delete(dv.headers, hd)
	return append(ready, w.header)
}

// drop forgets a header, once it is voted on.
func (dv *deferredVotes) drop(hd types.HeaderDigest) {
	dv.mu.Lock()
	defer dv.mu.Unlock()
	delete(dv.headers, hd)
}

// prune forgets headers below minRound, which no vote can be cast for, and
// index entries that name no recorded header.
func (dv *deferredVotes) prune(minRound types.Round) {
	dv.mu.Lock()
	defer dv.mu.Unlock()
	for hd, w := range dv.headers {
		if w.header.Round < minRound {
			delete(dv.headers, hd)
		}
	}
	for d, hds := range dv.byBatch {
		if !dv.anyRecorded(hds) {
			delete(dv.byBatch, d)
		}
	}
	for d, hds := range dv.byParent {
		if !dv.anyRecorded(hds) {
			delete(dv.byParent, d)
		}
	}
}

func (dv *deferredVotes) anyRecorded(hds []types.HeaderDigest) bool {
	for _, hd := range hds {
		if _, ok := dv.headers[hd]; ok {
			return true
		}
	}
	return false
}

// deferVote records what a header lacks and asks for it to be re-tested once
// it has all of it. What landed between the gate's check and the record is
// checked again here, so it is not waited for in vain.
func (p *Primary) deferVote(header *types.Header, parents []types.CertificateDigest, batches []types.BatchDigest) {
	p.deferred.wait(header, parents, batches)
	for _, d := range parents {
		if p.dag.GetByDigest(d) != nil {
			p.retest(p.deferred.parentArrived(d))
		}
	}
	for _, d := range batches {
		if p.haveBatch(d) {
			p.retest(p.deferred.batchArrived(d))
		}
	}
}

// BatchStored is told of each batch a worker stores (Worker.SetOnStored).
func (p *Primary) BatchStored(d types.BatchDigest) {
	p.retest(p.deferred.batchArrived(d))
}

// retest hands headers back to the primary's loop, which runs the vote gates
// as it does for a received header. A full queue drops them: the author's
// rebroadcast re-tests them within a second.
func (p *Primary) retest(headers []*types.Header) {
	for _, h := range headers {
		select {
		case p.retests <- h:
		default:
			slog.Debug("Deferred-vote re-test queue full; the rebroadcast will re-test it",
				"partition", p.config.Partition, "round", h.Round)
		}
	}
}

// pruneDeferredVotes forgets deferred votes for rounds no vote is accepted
// for any more, as the vote gate's round window does.
func (p *Primary) pruneDeferredVotes() {
	p.roundMu.Lock()
	current := p.currentRound
	p.roundMu.Unlock()
	if current > 1 {
		p.deferred.prune(current - 1)
	}
}
