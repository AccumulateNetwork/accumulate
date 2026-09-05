// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type interceptor = func(ctx context.Context, env *messaging.Envelope) (send bool, err error)

type Conductor struct {
	Partition    *protocol.PartitionInfo
	Globals      atomic.Pointer[network.GlobalValues]
	ValidatorKey ed25519.PrivateKey
	Database     database.Beginner
	Querier      api.Querier2
	Dispatcher   execute.Dispatcher

	// Sequencer serves the ranges recovery pulls. Without it a stalled stream
	// has no way back: dispatch is one-shot, so a message lost in transit is
	// never resent, and the destination's delivered-sequence stops there
	// permanently (#4105).
	Sequencer private.Sequencer

	// Staging is the executor's synthetic staging. The requesting side of
	// healing decides from it: an index above Delivered that staging does not
	// hold, or holds unproven, is a gap (healing.md, "Deciding, in staging").
	Staging *execute.Staging

	// Ready can be used to pause the conductor, for example to stop it from
	// sending anchors while the node is catching up.
	Ready func(execute.WillBeginBlock) bool

	// RunTask launches a background task. The caller may use this to wait for
	// completion of launched tasks.
	RunTask func(func())

	// **FOR TESTING PURPOSES ONLY**. Tells the conductor not to skip sending
	// the anchor the first time around.
	DropInitialAnchor bool

	// **FOR TESTING PURPOSES ONLY**. Intercepts dispatched envelopes.
	Intercept interceptor

	// HealTimeout is the deadline for a single healing scan, including its
	// queries to the destination. Defaults to DefaultHealTimeout.
	HealTimeout *time.Duration

	// Heals counts successful recoveries, so a node can report what it has had
	// to repair rather than only that it is currently healthy — the distinction
	// #4103 showed matters, where every surface said healthy while nothing was
	// being delivered.
	Heals *HealCounters

	// reconcileSeen is when a gap the SOURCE reported was first seen, so
	// reconcile can wait for it to persist rather than race normal delivery
	// (#4073). Not pacing: it is how long to disbelieve a remote's hint.
	synthHealMu   sync.Mutex
	reconcileSeen map[string]uint64
	synthHeals    atomic.Uint64

	// inflight is the per-task overlap guard (see runExclusive). It bounds what
	// healing may cost when a scan outlives the block that started it: without
	// it every block scheduled new scans regardless of whether the last had
	// finished, which is part of what drove the fleet to 17x CPU in the
	// 20260819T234054Z soak (#4115).
	//
	// The per-remote circuit breaker that used to sit beside it is gone
	// (#4201). It existed because every scan retried a failing remote at full
	// rate; with an activation every few blocks and two senders, there is no
	// rate to break.
	inflight sync.Map

	requester healRequester
}

// remoteHealth is one remote partition's circuit breaker state.
type remoteHealth struct {
	fails int       // consecutive pull/scan failures
	until time.Time // circuit open (skip this remote) until this time
}

// breakerThreshold is how many consecutive failures against a remote open its
// circuit, and breakerMax caps the backoff. Three failures is already three
// multi-second RPC timeouts — a remote that fails that consistently is down or
// drowning, and hammering it harder helps neither side.
const (
	breakerThreshold = 3
	breakerBase      = 15 * time.Second
	breakerMax       = 5 * time.Minute
)

// runExclusive runs the task like runTask, unless a task with the same key is
// still running — then it does nothing. Healing scans are scheduled from every
// block; a scan that outlives the block interval must not stack a second copy
// of itself on top.
// DefaultHealTimeout is the default deadline for a single healing activation.
const DefaultHealTimeout = 30 * time.Second

func (c *Conductor) runExclusive(key string, task func()) {
	if _, busy := c.inflight.LoadOrStore(key, struct{}{}); busy {
		return
	}
	c.runTask(func() {
		defer c.inflight.Delete(key)
		task()
	})
}

func (c *Conductor) Start(bus *events.Bus) error {
	events.SubscribeSync(bus, c.willBeginBlock)
	events.SubscribeSync(bus, c.willChangeGlobals)
	return nil
}

func (c *Conductor) Url(path ...string) *url.URL {
	return protocol.PartitionUrl(c.Partition.ID).JoinPath(path...)
}

func (c *Conductor) willChangeGlobals(e events.WillChangeGlobals) error {
	c.Globals.Store(e.New)
	return nil
}

func (c *Conductor) willBeginBlock(e execute.WillBeginBlock) error {
	// Skip if globals not yet loaded (fresh database before genesis)
	globals := c.Globals.Load()
	if globals == nil {
		return nil
	}

	// Skip for v1
	if !globals.ExecutorVersion.V2Enabled() {
		return nil
	}

	if c.Ready != nil && !c.Ready(e) {
		return nil
	}

	defer func() {
		errs := c.Dispatcher.Send(context.Background())
		c.runTask(func() {
			for err := range errs {
				switch err := err.(type) {
				case protocol.TransactionStatusError:
					slog.Error("Failed to dispatch transactions", "block", e.Index, "error", err, "stack", err.TransactionStatus.Error.PrintFullCallstack(), "txid", err.TxID)
				default:
					slog.Error("Failed to dispatch transactions", "block", e.Index, "error", fmt.Sprintf("%+v\n", err))
				}
			}
		})
	}()

	// Healing is staging's (healing.md): on the cadence, the destination
	// asks each source for the gaps its stages show, anchors and synthetics
	// alike. Nothing is pushed a second time from the source.
	activate := healActivates(e.Index)

	// Load the ledger state
	var ledger *protocol.SystemLedger
	batch := c.Database.Begin(false)
	defer batch.Discard()
	err := batch.Account(c.Url(protocol.Ledger)).Main().GetAs(&ledger)
	if err != nil {
		return errors.UnknownError.WithFormat("load system ledger: %w", err)
	}

	// Did anything happen last block?
	if activate && c.Staging != nil && c.Sequencer != nil && c.selectedToPull(ledger) {
		c.runExclusive("requestGaps", func() {
			ctx, cancel := context.WithTimeout(context.Background(), def(c.HealTimeout, DefaultHealTimeout))
			defer cancel()
			batch := c.Database.Begin(false)
			defer batch.Discard()
			err := c.requestGaps(ctx, batch, e.Index)
			if err != nil {
				slog.Error("Error while requesting missing synthetics", "error", err)
			}
		})
	}

	if ledger.Index < e.Index-1 {
		slog.DebugContext(e.Context, "Skipping anchor", "module", "conductor", "index", ledger.Index)
		return nil
	}

	// Send the anchor first, before synthetic transactions
	err = c.sendAnchorForLastBlock(e, batch)
	if err != nil {
		return errors.UnknownError.WithFormat("send anchor: %w", err)
	}

	// TODO Send synthetic transactions

	return nil
}

func (c *Conductor) sendAnchorForLastBlock(e execute.WillBeginBlock, batch *database.Batch) error {
	if c.DropInitialAnchor {
		return nil
	}

	// Construct the anchor
	anchor, sequenceNumber, err := ConstructLastAnchor(e.Context, batch, c.Url())
	if anchor == nil || err != nil {
		return errors.UnknownError.Wrap(err)
	}

	switch c.Partition.Type {
	case protocol.PartitionTypeDirectory:
		// DN -> all partitions
		for _, part := range c.Globals.Load().Network.Partitions {
			err = c.sendBlockAnchor(e.Context, anchor, sequenceNumber, part.ID)
			if err != nil {
				return errors.UnknownError.WithFormat("send anchor: %w", err)
			}
		}

	case protocol.PartitionTypeBlockValidator:
		// BVN -> DN
		err = c.sendBlockAnchor(e.Context, anchor, sequenceNumber, protocol.Directory)
		if err != nil {
			return errors.UnknownError.WithFormat("send anchor: %w", err)
		}
	}
	return nil
}

func (c *Conductor) sendBlockAnchor(ctx context.Context, anchor protocol.AnchorBody, sequenceNumber uint64, destPart string) error {
	destination := protocol.PartitionUrl(destPart)
	// Info, not Debug, and with the sequence number: tracing one lost anchor
	// signature (#4111) requires seeing every validator's send for a given seq.
	slog.InfoContext(ctx, "Sending an anchor", "module", "conductor",
		"block", anchor.GetPartitionAnchor().MinorBlockIndex,
		"destination", destination,
		"seq", sequenceNumber,
		"root", logging.AsHex(anchor.GetPartitionAnchor().RootChainAnchor).Slice(0, 4),
		"bpt", logging.AsHex(anchor.GetPartitionAnchor().StateTreeAnchor).Slice(0, 4))

	// Construct the envelope
	env, _, err := ValidatorContext{
		Source:       c.Partition,
		Globals:      c.Globals.Load(),
		ValidatorKey: c.ValidatorKey,
	}.PrepareAnchorSubmission(ctx, anchor, sequenceNumber, destination)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Submit it
	return c.submit(ctx, destination, env)
}

func (c *Conductor) submit(ctx context.Context, url *url.URL, env *messaging.Envelope) error {
	if c.Intercept != nil {
		keep, err := c.Intercept(ctx, env)
		if !keep || err != nil {
			return err
		}
	}

	return c.Dispatcher.Submit(ctx, url, env)
}

func (c *Conductor) runTask(task func()) {
	if c.RunTask != nil {
		c.RunTask(task)
		return
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("Background task panicked", "error", r, "stack", debug.Stack())
			}
		}()

		task()
	}()
}

func def[T any](value *T, def T) T {
	if value == nil {
		return def
	}
	return *value
}

// HealCounters is shared with the consensus service so recoveries are visible
// to operators rather than only in logs.
type HealCounters struct {
	Synthetic atomic.Uint64 // synthetic entries a requested bundle carried
	Anchor    atomic.Uint64
	Requests  atomic.Uint64 // span requests sent
	Misses    atomic.Uint64 // requests the source could not answer from its cache
}
