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
	dest   string
	remain atomic.Int32
}

// parseDropSyntheticSpec parses "<partition>:<count>" (count defaults to 1).
// Returns nil if the spec is empty or malformed.
func parseDropSyntheticSpec(spec string) *synthDropper {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil
	}
	dest, countStr, hasCount := strings.Cut(spec, ":")
	count := 1
	if hasCount {
		n, err := strconv.Atoi(strings.TrimSpace(countStr))
		if err != nil || n < 0 {
			slog.Error("Ignoring malformed ACC_DEBUG_DROP_SYNTHETIC", "spec", spec)
			return nil
		}
		count = n
	}
	d := &synthDropper{dest: strings.TrimSpace(dest)}
	d.remain.Store(int32(count))
	slog.Warn("DEBUG synthetic-drop enabled — reproducing a wedged synthetic stream",
		"destination", d.dest, "count", count)
	return d
}

// tryDrop reports whether this synthetic envelope to dest should be dropped,
// consuming one of the remaining drops if so.
func (d *synthDropper) tryDrop(dest *url.URL, env *messaging.Envelope) bool {
	if !d.matches(dest) || !envHasSynthetic(env) {
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
	dropper *synthDropper
}

func (d *droppingDispatcher) Submit(ctx context.Context, dest *url.URL, env *messaging.Envelope) error {
	if d.dropper.tryDrop(dest, env) {
		return nil // silently drop
	}
	return d.Dispatcher.Submit(ctx, dest, env)
}
