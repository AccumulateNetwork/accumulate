// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"encoding/hex"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Why a held head may not run: the conditions of [streamPosition.runnable]
// that failed, one name each (#4465).
const (
	// headNotValidated: collected without a proof of its own, and no hash is
	// validated at its number. It waits for a package proof over it.
	headNotValidated = "not-validated"
	// headValidatedOther: a hash IS validated at its number, and it is not
	// the held one. Nothing will make the held entry runnable.
	headValidatedOther = "validated-hash-differs"
	// headOwnProofAnchorAbsent: collected with its own proof, whose Directory
	// anchor is not on this node's Directory anchor chain.
	headOwnProofAnchorAbsent = "own-proof-anchor-not-executed"
	// headBelowQuorum: an anchor whose validator signatures are below quorum.
	headBelowQuorum = "anchor-below-quorum"
	// headError: the check itself failed.
	headError = "error"
)

// headBlock is what a held, unrunnable number looked like when it was refused.
type headBlock struct {
	number        uint64
	reason        string
	held          [32]byte
	validated     bool
	validatedHash [32]byte

	ownProof         bool
	proofAnchor      []byte
	proofAnchorBlock uint64
	err              error
}

// settle names the reason: a failed check is an error whatever it was asking,
// and a different validated hash outranks every other reason, because it is
// the one no arrival can clear.
func (h *headBlock) settle() *headBlock {
	switch {
	case h.err != nil:
		h.reason = headError
	case h.validated && h.validatedHash != h.held:
		h.reason = headValidatedOther
	}
	return h
}

// mExecHeadBlocked counts the stages at which a stream's next number was held
// and not runnable, by stream kind and reason. A stream held this way with
// waiting=0 is the #4465 wedge: everything received, nothing executed.
var mExecHeadBlocked = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "accumulate",
	Subsystem: "exec",
	Name:      "head_blocked_total",
	Help:      "Stages at which a stream's next number was held but not runnable, by stream kind (synthetic, anchor) and reason",
}, []string{"stream", "reason"})

// HeadBlockedEvery is how often a stream blocked at the same number for the
// same reason is logged again, in block time.
const HeadBlockedEvery = time.Minute

// headBlockLogState is the last `Stream head blocked` line per stream.
type headBlockLogState struct {
	mu   sync.Mutex
	last map[string]headBlockLogEntry
}

type headBlockLogEntry struct {
	number uint64
	reason string
	at     time.Time
}

// noteHeadBlocked records a stream whose next number is held and may not run:
// counted every time, logged once per stream, number and reason, and again no
// more often than HeadBlockedEvery while it stays so.
func (p *streamPosition) noteHeadBlocked(why *headBlock) {
	b := p.block
	if b == nil {
		return
	}
	kind := "synthetic"
	if p.stream.kind == streamAnchor {
		kind = "anchor"
	}
	mExecHeadBlocked.WithLabelValues(kind, why.reason).Inc()

	l := &b.Executor.headBlockLog
	k := p.stream.key()
	l.mu.Lock()
	prev, seen := l.last[k]
	if seen && prev.number == why.number && prev.reason == why.reason && b.Time.Sub(prev.at) < HeadBlockedEvery {
		l.mu.Unlock()
		return
	}
	if l.last == nil {
		l.last = map[string]headBlockLogEntry{}
	}
	l.last[k] = headBlockLogEntry{number: why.number, reason: why.reason, at: b.Time}
	l.mu.Unlock()

	validated := "none"
	if why.validated {
		validated = hex.EncodeToString(why.validatedHash[:])
	}
	kv := []any{"block", b.Index, "ledger", kind, "source", p.stream.source,
		"number", why.number, "delivered", p.delivered, "received", p.received(),
		"reason", why.reason,
		"held", hex.EncodeToString(why.held[:]),
		"validated", validated,
		"own-proof", why.ownProof,
		"module", "stream"}
	if why.ownProof {
		// Named, and not found on this node's Directory anchor chain: that is
		// what own-proof-anchor-not-executed means
		kv = append(kv, "proof-anchor", hex.EncodeToString(why.proofAnchor),
			"proof-anchor-block", why.proofAnchorBlock, "anchor-executed", false)
	}
	if why.err != nil {
		kv = append(kv, "error", why.err)
	}
	b.Executor.logger.Info("Stream head blocked", kv...)
}
