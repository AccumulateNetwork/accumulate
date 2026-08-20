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
	"strings"
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

	// Ready can be used to pause the conductor, for example to stop it from
	// sending anchors while the node is catching up.
	Ready func(execute.WillBeginBlock) bool

	// RunTask launches a background task. The caller may use this to wait for
	// completion of launched tasks.
	RunTask func(func())

	// **FOR TESTING PURPOSES ONLY**. Tells the conductor not to skip sending
	// the anchor the first time around.
	DropInitialAnchor bool

	// Enables healing of anchors after they are initially submitted.
	EnableAnchorHealing *bool

	// **FOR TESTING PURPOSES ONLY**. Intercepts dispatched envelopes.
	Intercept interceptor

	// HealInterval is the minimum time between anchor-healing scans per
	// destination. Healing fires from WillBeginBlock; with CometBFT that is
	// ~1 block/s but DAG-BFT produces a block per committed certificate —
	// dozens per second — and each scan queries the destination partition,
	// so healing must be paced independently of block cadence. Defaults to
	// DefaultHealInterval.
	HealInterval time.Duration

	// HealTimeout is the deadline for a single healing scan, including its
	// queries to the destination. Defaults to DefaultHealTimeout.
	HealTimeout *time.Duration

	// lastHeal tracks the last healing scan per destination.
	lastHealMu sync.Mutex
	lastHeal   map[string]time.Time

	// Heals counts successful recoveries, so a node can report what it has had
	// to repair rather than only that it is currently healthy — the distinction
	// #4103 showed matters, where every surface said healthy while nothing was
	// being delivered.
	Heals *HealCounters

	// SyntheticHealWindow overrides the jitter/back-off window for synthetic
	// recovery. Zero uses the default. Tests set a small value because
	// simulator blocks are not wall-clock paced.
	SyntheticHealWindow time.Duration

	// Recovery state, mirroring the anchor side's lastHeal pacing (#4105).
	synthHealMu    sync.Mutex
	synthHealState map[string]*synthHealEntry
	seqHealAt      map[string]time.Time // per-(source,seq) last heal submission
	reconcileSeen  map[string]uint64    // per-(source,seq) block a gap was first seen
	synthHeals     atomic.Uint64

	// delivery tracks each destination's anchor-delivery progress across scans,
	// so healing acts only when delivery is genuinely stalled rather than
	// merely catching up or momentarily paused.
	deliveryMu sync.Mutex
	delivery   map[string]*deliveryProgress

	// remotes carries the per-remote circuit breaker (see remoteAllowed), and
	// inflight the per-task overlap guard (see runExclusive). Together they
	// bound what healing may cost: without them the healers were the single
	// largest load source in the 20260819T234054Z soak — every block scheduled
	// new scans regardless of whether the last had finished, and every scan
	// retried a failing remote at full rate, which is what drove the fleet to
	// 17x CPU and exhausted the libp2p stream budget (#4115).
	remoteMu sync.Mutex
	remotes  map[string]*remoteHealth
	inflight sync.Map
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

// remoteAllowed reports whether pull-healing may talk to the given remote, or
// whether its circuit is open after repeated failures.
func (c *Conductor) remoteAllowed(remote string) bool {
	c.remoteMu.Lock()
	defer c.remoteMu.Unlock()
	r := c.remotes[remote]
	return r == nil || time.Now().After(r.until)
}

// remoteOK records a successful interaction and closes the remote's circuit.
func (c *Conductor) remoteOK(remote string) {
	c.remoteMu.Lock()
	defer c.remoteMu.Unlock()
	if r := c.remotes[remote]; r != nil {
		r.fails, r.until = 0, time.Time{}
	}
}

// classifyRemoteError feeds the circuit breaker only for errors that say
// something about the REMOTE's health. A deterministic "cannot serve yet" —
// notFound (the servable chain has not reached the requested entry, #4086) or
// notReady (the covering anchor has not executed at the destination yet) — is
// a healthy remote giving a correct answer, and counting it opened the shared
// per-remote breaker, which then blocked ANCHOR healing to that remote even
// though anchor recovery is self-contained and would have succeeded. Run
// 20260820T073651Z: BVN1's anchor delivery trickled at ~2 per 20 minutes
// because synthetic-heal notReady answers kept the breaker open.
func (c *Conductor) classifyRemoteError(remote string, err error) {
	if errors.Is(err, errors.NotFound) || errors.Is(err, errors.NotReady) {
		c.remoteOK(remote)
		return
	}
	c.remoteFailed(remote)
}

// remoteFailed records a failed interaction; after breakerThreshold
// consecutive failures the remote's circuit opens with exponential backoff.
func (c *Conductor) remoteFailed(remote string) {
	c.remoteMu.Lock()
	defer c.remoteMu.Unlock()
	if c.remotes == nil {
		c.remotes = make(map[string]*remoteHealth)
	}
	r := c.remotes[remote]
	if r == nil {
		r = new(remoteHealth)
		c.remotes[remote] = r
	}
	r.fails++
	if r.fails < breakerThreshold {
		return
	}
	d := breakerBase << (r.fails - breakerThreshold)
	if d > breakerMax || d <= 0 {
		d = breakerMax
	}
	r.until = time.Now().Add(d)
	slog.Info("Healing circuit open", "module", "conductor",
		"remote", remote, "consecutiveFailures", r.fails, "retryIn", d)
}

// runExclusive runs the task like runTask, unless a task with the same key is
// still running — then it does nothing. Healing scans are scheduled from every
// block; a scan that outlives the block interval must not stack a second copy
// of itself on top.
func (c *Conductor) runExclusive(key string, task func()) {
	if _, busy := c.inflight.LoadOrStore(key, struct{}{}); busy {
		return
	}
	c.runTask(func() {
		defer c.inflight.Delete(key)
		task()
	})
}

// StallScans is the number of consecutive scans a destination's delivered
// anchor count must fail to advance, while anchors remain undelivered, before
// its next anchor is treated as stuck and resubmitted. At DefaultHealInterval
// this is a ~30s window — long enough that normal, bursty delivery (which
// pauses for a scan or two between batches) is not mistaken for a stall, short
// enough to recover a genuinely lost quorum promptly.
const StallScans = 3

type deliveryProgress struct {
	delivered uint64 // delivered count at the last scan
	stalls    int    // consecutive scans with no advance while behind
}

// DefaultHealInterval is the default minimum time between anchor-healing
// scans per destination.
const DefaultHealInterval = 10 * time.Second

// DefaultHealTimeout is the default deadline for a single healing scan.
const DefaultHealTimeout = 30 * time.Second

// shouldHeal returns true if enough time has passed since the last healing
// scan for the given destination, and records the scan time.
func (c *Conductor) shouldHeal(destination string) bool {
	interval := c.HealInterval
	if interval <= 0 {
		interval = DefaultHealInterval
	}

	c.lastHealMu.Lock()
	defer c.lastHealMu.Unlock()

	if c.lastHeal == nil {
		c.lastHeal = make(map[string]time.Time)
	}
	if time.Since(c.lastHeal[destination]) < interval {
		return false
	}
	c.lastHeal[destination] = time.Now()
	return true
}

// deliveryStalled reports whether the destination's delivered anchor count has
// failed to advance across StallScans consecutive scans while anchors remain
// undelivered. Anchors deliver sequentially, so a destination whose Delivered
// is climbing is flowing on its own and needs no help — re-driving its in-flight
// anchors only adds load, and under DAG-BFT's dozens-of-blocks-per-second
// cadence that is the feedback that saturates a partition. Delivery is bursty
// (a batch, then a pause), so a single stalled scan is not enough to conclude a
// stall; only Delivered pinned across the whole window marks the next anchor as
// genuinely stuck — a quorum lost to validator churn, say — and worth a
// resubmission (#4056). It updates the destination's progress each call.
func (c *Conductor) deliveryStalled(destination string, delivered, produced uint64) bool {
	c.deliveryMu.Lock()
	defer c.deliveryMu.Unlock()

	if c.delivery == nil {
		c.delivery = make(map[string]*deliveryProgress)
	}
	p := c.delivery[destination]
	if p == nil {
		p = new(deliveryProgress)
		c.delivery[destination] = p
	}

	switch {
	case delivered >= produced:
		// Caught up — nothing undelivered
		p.delivered, p.stalls = delivered, 0
		return false
	case delivered > p.delivered:
		// Advancing — flowing on its own, reset the stall count
		p.delivered, p.stalls = delivered, 0
		return false
	default:
		// Behind and not advancing this scan — stuck only once the count has
		// been pinned across the whole window
		p.stalls++
		return p.stalls >= StallScans
	}
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

	// Check old anchors. Healing queries the DESTINATION, so every scan gets
	// a deadline: an unreachable or restarted destination (stale peer IDs)
	// otherwise hangs the query forever — the goroutine leaks silently and
	// that destination is never healed again (#4056).
	healOne := func(destination *url.URL) {
		if !c.remoteAllowed(destination.String()) {
			return
		}
		c.runExclusive("healAnchors:"+destination.String(), func() {
			ctx, cancel := context.WithTimeout(context.Background(), def(c.HealTimeout, DefaultHealTimeout))
			defer cancel()

			batch := c.Database.Begin(false)
			defer batch.Discard()

			err := c.healAnchors(ctx, batch, destination, e.Index)
			if err != nil {
				c.classifyRemoteError(destination.String(), err)
				slog.Error("Error while healing anchors", "destination", destination, "error", err)
			} else {
				c.remoteOK(destination.String())
			}
		})
	}
	// Destination-side recovery. This is the half healAnchors defers to when
	// collection proofs are active: it retires the source-side push on the
	// grounds that "the DESTINATION owns anchor recovery", and until now that
	// owner did not exist on this branch — so under Kourou nothing retried at
	// all and a single lost message wedged a stream permanently (#4103, #4105).
	//
	// Paced by the same shouldHeal window as the push it replaces, so the
	// StallScans reasoning about DAG-BFT block rates still governs how often a
	// destination asks.
	// Request any missing inbound synthetic messages (receiver-pull on gap).
	// Unconditional: a lost synthetic wedges the stream permanently, so recovery
	// is not something an operator should be able to switch off.
	if c.Sequencer != nil {
		// Exclusive: this is scheduled every block, and at a short block
		// interval a scan over many gapped streams outlives the block. Copies
		// used to stack without bound (#4115).
		c.runExclusive("requestMissingSynthetics", func() {
			batch := c.Database.Begin(false)
			defer batch.Discard()

			err := c.requestMissingSynthetics(context.Background(), batch)
			if err != nil {
				slog.Error("Error while requesting missing synthetics", "error", err)
			}
		})
	}

	if c.Sequencer != nil {
		for _, src := range c.Globals.Load().Network.Partitions {
			if strings.EqualFold(src.ID, c.Partition.ID) {
				continue
			}
			if !c.shouldHeal("recover:" + src.ID) {
				continue
			}
			source := protocol.PartitionUrl(src.ID)
			if !c.remoteAllowed(source.String()) {
				continue
			}
			c.runExclusive("recoverAnchors:"+source.String(), func() {
				ctx, cancel := context.WithTimeout(context.Background(), def(c.HealTimeout, DefaultHealTimeout))
				defer cancel()

				batch := c.Database.Begin(false)
				defer batch.Discard()

				err := c.recoverAnchorsViaRange(ctx, batch, source)
				if err != nil {
					c.classifyRemoteError(source.String(), err)
					slog.Error("Error while recovering anchors by range", "source", src.ID, "error", err)
				} else {
					c.remoteOK(source.String())
				}
			})
		}

		// The "anything new?" pull. A gap the destination can SEE is bounded by
		// the entry that exposed it; a stream whose tail was lost shows no gap
		// at all, and only asking the source what it has produced finds it.
		if e.Index%reconcileInterval == 0 {
			c.runTask(func() {
				batch := c.Database.Begin(false)
				defer batch.Discard()

				err := c.reconcileInboundStreams(context.Background(), batch, e.Index)
				if err != nil {
					slog.Error("Error while reconciling inbound streams", "error", err)
				}
			})
		}
	}

	if c.Partition.Type != protocol.PartitionTypeDirectory {
		if c.shouldHeal(protocol.Directory) {
			healOne(protocol.DnUrl())
		}
	} else {
		for _, dst := range c.Globals.Load().Network.Partitions {
			if c.shouldHeal(dst.ID) {
				healOne(protocol.PartitionUrl(dst.ID))
			}
		}
	}

	// Load the ledger state
	var ledger *protocol.SystemLedger
	batch := c.Database.Begin(false)
	defer batch.Discard()
	err := batch.Account(c.Url(protocol.Ledger)).Main().GetAs(&ledger)
	if err != nil {
		return errors.UnknownError.WithFormat("load system ledger: %w", err)
	}

	// Did anything happen last block?
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
	slog.DebugContext(ctx, "Sending an anchor", "module", "conductor",
		"block", anchor.GetPartitionAnchor().MinorBlockIndex,
		"destination", destination,
		"source-block", anchor.GetPartitionAnchor().MinorBlockIndex,
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
	Synthetic atomic.Uint64
	Anchor    atomic.Uint64
}
