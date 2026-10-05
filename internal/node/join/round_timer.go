// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"log/slog"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// slowRound is how long a pull round runs before it says where its time went.
const slowRound = 10 * time.Second

// slowCall is how long one call to a peer or validator runs before it is said.
const slowCall = 5 * time.Second

// roundTimer is where one pull round's time went, phase by phase: the one line
// a post mortem of a slow join needs (#4524). A round that ends within
// slowRound says nothing.
type roundTimer struct {
	start, last time.Time
	args        []any
}

func newRoundTimer() *roundTimer {
	now := time.Now()
	return &roundTimer{start: now, last: now}
}

// mark ends the phase named, from the last mark.
func (r *roundTimer) mark(phase string) {
	now := time.Now()
	r.args = append(r.args, phase, now.Sub(r.last).Round(time.Millisecond))
	r.last = now
}

func (r *roundTimer) report(log *slog.Logger, partition *url.URL) {
	total := time.Since(r.start)
	if total < slowRound {
		return
	}
	args := append([]any{"partition", partition, "total", total.Round(time.Millisecond)}, r.args...)
	if rest := time.Since(r.last); rest > time.Millisecond {
		args = append(args, "unmarked", rest.Round(time.Millisecond))
	}
	log.Warn("A pull round was slow; where its time went", args...)
}
