// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A Collector takes healed packages into staging outside any block: what a
// node rejoining after a restart does with the answers it pulls before it
// executes its first block (executor spec, "Sync").
type Collector interface {
	Collect(batch *database.Batch, envelopes []*messaging.Envelope) (int, error)
}

// rejoinRetries and rejoinRetryWait bound how long a rejoining node waits for
// a source that does not answer: a node that has just started may not have
// its peers yet, and the pull is the one thing the block waits for. A minute
// in all; after that the stream is logged as failed and the block runs.
const (
	rejoinRetries   = 12
	rejoinRetryWait = 5 * time.Second
)

// maxRejoinSpans bounds one stream's rejoin walk, in spans of
// MaxReceiptListElements from Delivered up. A node further behind than that
// is beyond what healing rebuilds and needs sync (E11, #4205).
const maxRejoinSpans = 64

var mRejoinSpans = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "accumulate",
	Subsystem: "conductor",
	Name:      "rejoin_spans_total",
	Help:      "Spans a rejoining node asked its sources for before its first block, by outcome: answered (held), not-yet (the source has produced nothing more: the stream is rebuilt), miss (the source's cache no longer holds what this node's peers hold: it cannot rejoin by healing and needs sync, #4290), failed",
}, []string{"outcome", "destination", "source"})

// Rejoin marks the node as rejoining: at its next block-begin, before that
// block executes, every inbound synthetic stream is pulled from its source
// from Delivered up and what comes back is held. Staging is memory and a
// restart empties it, while the node's peers keep what they hold and execute
// it the moment its proof's anchor lands; a node that executed that block
// without holding the same would diverge from them for good (#4290). Start
// calls it; a test calls it to stand a node where a restarted node stands.
//
// Anchor streams need no rebuilding: a block anchor copy's signature is
// recorded in the store as it arrives, so the quorum a restarted node was
// gathering is still there.
func (c *Conductor) Rejoin() { c.rejoinPending.Store(true) }

func (c *Conductor) rejoin(blockIndex uint64) {
	ranger, ok := c.Sequencer.(private.SequenceRanger)
	if !ok || c.Collector == nil || c.Staging == nil {
		return
	}
	batch := c.Database.Begin(false)
	defer batch.Discard()
	// A node that has executed no block yet is starting at genesis, not
	// restarting: it held nothing.
	var system *protocol.SystemLedger
	switch err := batch.Account(c.Url(protocol.Ledger)).Main().GetAs(&system); {
	case errors.Is(err, errors.NotFound):
		return
	case err != nil:
		slog.Error("Cannot rejoin: system ledger", "module", "conductor", "destination", c.Url(), "error", err)
		return
	}
	if system.Index == 0 {
		return
	}
	var synth *protocol.SyntheticLedger
	switch err := batch.Account(c.Url(protocol.Synthetic)).Main().GetAs(&synth); {
	case errors.Is(err, errors.NotFound):
		return // nothing was ever delivered: nothing to rebuild
	case err != nil:
		slog.Error("Cannot rejoin: synthetic ledger", "module", "conductor", "destination", c.Url(), "error", err)
		return
	}
	for _, source := range c.inboundSources(synth) {
		delivered := synth.Partition(source).Delivered
		first := delivered + 1
		held, spans, retries := 0, 0, 0
		outcome := "complete"
	walk:
		for ; spans < maxRejoinSpans; spans++ {
			var envs []*messaging.Envelope
			sink := func(env *messaging.Envelope) error { envs = append(envs, env); return nil }
			ctx, cancel := context.WithTimeout(context.Background(), def(c.HealTimeout, DefaultHealTimeout))
			_, served, err := c.requestSpanTo(ctx, ranger, source, first, first+protocol.MaxReceiptListElements-1,
				func(uint64) string { return "rejoined" }, sink)
			cancel()
			switch {
			case err == nil:
				n, err := c.Collector.Collect(batch, envs)
				if err != nil {
					outcome = "failed"
					mRejoinSpans.WithLabelValues("failed", c.Partition.ID, partitionLabel(source)).Inc()
					slog.Error("Cannot rejoin: intake failed", "module", "conductor", "source", source, "destination", c.Url(), "start", first, "error", err)
					break walk
				}
				held += n
				mRejoinSpans.WithLabelValues("answered", c.Partition.ID, partitionLabel(source)).Inc()
				if served < first {
					break walk
				}
				first = served + 1
			case errors.Is(err, errors.NotReady):
				// The source has produced nothing more, or its tail is not
				// yet provable and so not yet dispatched: the peers hold
				// nothing beyond this either.
				mRejoinSpans.WithLabelValues("not-yet", c.Partition.ID, partitionLabel(source)).Inc()
				break walk
			case errors.Is(err, errors.NotFound):
				outcome = "stranded"
				mRejoinSpans.WithLabelValues("miss", c.Partition.ID, partitionLabel(source)).Inc()
				slog.Error("Cannot rejoin by healing: the source no longer holds what this node's peers hold; this node's state will diverge until it syncs (#4290, #4205)",
					"module", "conductor", "source", source, "destination", c.Url(), "start", first, "delivered", delivered, "error", err)
				break walk
			default:
				if retries < rejoinRetries {
					retries++
					spans--
					slog.Warn("Rejoin request failed; asking again", "module", "conductor", "source", source, "destination", c.Url(), "start", first, "attempt", retries, "error", err)
					time.Sleep(c.rejoinWait())
					continue
				}
				outcome = "failed"
				mRejoinSpans.WithLabelValues("failed", c.Partition.ID, partitionLabel(source)).Inc()
				slog.Error("Cannot rejoin: request failed", "module", "conductor", "source", source, "destination", c.Url(), "start", first, "error", err)
				break walk
			}
		}
		if spans == maxRejoinSpans {
			outcome = "incomplete"
		}
		if delivered == 0 && held == 0 && outcome == "complete" {
			continue // a stream nothing has ever come down: not worth a line
		}
		slog.Info("Rejoined stream", "module", "conductor", "source", source, "destination", c.Url(),
			"delivered", delivered, "held", held, "spans", spans, "outcome", outcome, "block", blockIndex)
	}
}

// rejoinWait is the pause between retries: none under a test's HealTimeout
// of zero, which stands for "do not wait on the network".
func (c *Conductor) rejoinWait() time.Duration {
	if c.HealTimeout != nil && *c.HealTimeout == 0 {
		return 0
	}
	return rejoinRetryWait
}
