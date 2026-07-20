// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// synthDropper holds the shared "drop the first N synthetics to <dest>" state.
// It exists ONLY to reproduce a stalled synthetic stream (#4064) in a real
// network — production has no automatic synthetic retry, so a single dropped
// message wedges every later one. Enabled via
// ACC_DEBUG_DROP_SYNTHETIC=<partition>:<count>; a no-op when unset.
type synthDropper struct {
	dest    string
	anchors bool // match anchor envelopes instead of synthetics
	remain  atomic.Int32

	// Modulo mode ("<dest>:%K"): drop the FIRST submission of any message
	// whose sequence number is divisible by K. Because everything is
	// sequenced, every validator evaluates the same rule and drops the same
	// message — a guaranteed real wedge with no coordination — while the
	// healer's re-submission of the same sequence number passes (each seq is
	// dropped only once per node). Recurs throughout a long soak (#4067).
	modulo  uint64
	run     uint64 // drop seqs where seq%modulo < run (a RUN of consecutive losses)
	mu      sync.Mutex
	dropped map[uint64]bool
}

// parseDropSyntheticSpec parses "<partition>:<count>" (count defaults to 1).
// Returns nil if the spec is empty or malformed.
func parseDropSyntheticSpec(spec string) *synthDropper {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil
	}
	dest, countStr, hasCount := strings.Cut(spec, ":")
	d := &synthDropper{dest: strings.TrimSpace(dest)}
	countStr = strings.TrimSpace(countStr)
	if hasCount && strings.HasPrefix(countStr, "%") {
		kStr, rStr, hasRun := strings.Cut(countStr[1:], "+")
		k, err := strconv.ParseUint(kStr, 10, 64)
		if err != nil || k == 0 {
			slog.Error("Ignoring malformed drop spec", "spec", spec)
			return nil
		}
		d.run = 1
		if hasRun {
			r, err := strconv.ParseUint(rStr, 10, 64)
			if err != nil || r == 0 || r >= k {
				slog.Error("Ignoring malformed drop spec", "spec", spec)
				return nil
			}
			d.run = r
		}
		d.modulo = k
		d.dropped = map[uint64]bool{}
		slog.Warn("DEBUG sequence-drop enabled — every Kth sequence dropped once",
			"destination", d.dest, "modulo", k)
		return d
	}
	count := 1
	if hasCount {
		n, err := strconv.Atoi(countStr)
		if err != nil || n < 0 {
			slog.Error("Ignoring malformed drop spec", "spec", spec)
			return nil
		}
		count = n
	}
	d.remain.Store(int32(count))
	slog.Warn("DEBUG synthetic-drop enabled — reproducing a wedged synthetic stream",
		"destination", d.dest, "count", count)
	return d
}

// tryDrop reports whether this synthetic envelope to dest should be dropped,
// consuming one of the remaining drops if so.
func (d *synthDropper) tryDrop(dest *url.URL, env *messaging.Envelope) bool {
	if !d.matches(dest) {
		return false
	}
	if d.modulo > 0 {
		seq, ok := envSequenceNumber(env, d.anchors)
		if !ok || seq%d.modulo >= d.run {
			return false
		}
		d.mu.Lock()
		already := d.dropped[seq]
		d.dropped[seq] = true
		d.mu.Unlock()
		if already {
			return false // let the healed re-submission through
		}
		slog.Warn("DEBUG dropping sequenced envelope", "destination", dest, "sequence", seq, "anchor", d.anchors)
		return true
	}
	if !envHasSynthetic(env) {
		return false
	}
	for {
		n := d.remain.Load()
		if n <= 0 {
			return false
		}
		if d.remain.CompareAndSwap(n, n-1) {
			slog.Warn("DEBUG dropping synthetic envelope", "destination", dest, "remaining", n-1)
			return true
		}
	}
}

func (d *synthDropper) matches(dest *url.URL) bool {
	if dest == nil {
		return false
	}
	if d.dest == "*" {
		return true // any destination
	}
	if id, ok := protocol.ParsePartitionUrl(dest); ok {
		return strings.EqualFold(id, d.dest)
	}
	return strings.EqualFold(dest.String(), d.dest)
}

// envSequenceNumber extracts the sequence number of the envelope's synthetic
// or anchor message.
func envSequenceNumber(env *messaging.Envelope, anchors bool) (uint64, bool) {
	for _, msg := range env.Messages {
		switch m := msg.(type) {
		case *messaging.SyntheticMessage:
			if !anchors {
				if seq, ok := m.Message.(*messaging.SequencedMessage); ok {
					return seq.Number, true
				}
			}
		case *messaging.BadSyntheticMessage:
			if !anchors {
				if seq, ok := m.Message.(*messaging.SequencedMessage); ok {
					return seq.Number, true
				}
			}
		case *messaging.BlockAnchor:
			if anchors {
				if seq, ok := m.Anchor.(*messaging.SequencedMessage); ok {
					return seq.Number, true
				}
			}
		}
	}
	return 0, false
}

func envHasSynthetic(env *messaging.Envelope) bool {
	for _, msg := range env.Messages {
		switch msg.(type) {
		case *messaging.SyntheticMessage, *messaging.BadSyntheticMessage:
			return true
		}
	}
	return false
}

// droppingDispatcher wraps a dispatcher and drops synthetics per its shared
// synthDropper.
type droppingDispatcher struct {
	execute.Dispatcher
	dropper       *synthDropper // synthetics
	anchorDropper *synthDropper // anchors
}

func (d *droppingDispatcher) Submit(ctx context.Context, dest *url.URL, env *messaging.Envelope) error {
	if d.dropper != nil && d.dropper.tryDrop(dest, env) {
		return nil // silently drop
	}
	if d.anchorDropper != nil && d.anchorDropper.tryDrop(dest, env) {
		return nil // silently drop
	}
	return d.Dispatcher.Submit(ctx, dest, env)
}
