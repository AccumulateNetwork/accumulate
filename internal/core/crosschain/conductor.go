// Copyright 2025 The Accumulate Authors
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
	"sync/atomic"

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
	// Skip for v1
	if !c.Globals.Load().ExecutorVersion.V2Enabled() {
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

	// Check old anchors
	if c.Partition.Type != protocol.PartitionTypeDirectory {
		c.runTask(func() {
			batch := c.Database.Begin(false)
			defer batch.Discard()

			err := c.healAnchors(context.Background(), batch, protocol.DnUrl(), e.Index)
			if err != nil {
				slog.Error("Error while healing anchors", "error", err)
			}
		})

	} else {
		for _, dst := range c.Globals.Load().Network.Partitions {
			dst := dst
			c.runTask(func() {
				batch := c.Database.Begin(false)
				defer batch.Discard()

				err := c.healAnchors(context.Background(), batch, protocol.PartitionUrl(dst.ID), e.Index)
				if err != nil {
					slog.Error("Error while healing anchors", "error", err)
				}
			})
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
