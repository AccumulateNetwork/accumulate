// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bsn

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Collector struct {
	partition string
	db        database.Beginner
	events    *events.Bus
	previous  chan uint64
	latest    chan *messaging.BlockSummary
}

type CollectorOptions struct {
	Partition string
	Database  database.Beginner
	Events    *events.Bus
}

type DidCollectBlock struct {
	Summary *messaging.BlockSummary
}

func StartCollector(opts CollectorOptions) (*Collector, error) {
	c := new(Collector)
	c.partition = opts.Partition
	c.db = opts.Database
	c.events = opts.Events
	c.previous = make(chan uint64, 1)
	c.latest = make(chan *messaging.BlockSummary, 1)
	events.SubscribeSync(opts.Events, c.willBeginBlock)
	events.SubscribeSync(opts.Events, c.willCommitBlock)
	events.SubscribeSync(opts.Events, c.didCommitBlock)

	// Load the index of the last block
	batch := c.db.Begin(false)
	defer batch.Discard()
	var ledger *protocol.SystemLedger
	err := batch.Account(protocol.PartitionUrl(c.partition).JoinPath(protocol.Ledger)).Main().GetAs(&ledger)
	switch {
	case err == nil:
		c.previous <- ledger.Index
	case errors.Is(err, errors.NotFound):
		c.previous <- 0
	default:
		return nil, errors.UnknownError.WithFormat("load system ledger: %w", err)
	}

	return c, nil
}

func (DidCollectBlock) IsEvent() {}

func (c *Collector) willBeginBlock(e execute.WillBeginBlock) error {
	batch := c.db.Begin(false)
	defer batch.Discard()
	var ledger *protocol.SystemLedger
	err := batch.Account(protocol.PartitionUrl(c.partition).JoinPath(protocol.Ledger)).Main().GetAs(&ledger)
	if err != nil {
		return errors.UnknownError.WithFormat("load system ledger: %w", err)
	}

	// Drain the channel
	select {
	case <-c.previous:
	default:
	}

	// Send the index of the previous block
	select {
	case c.previous <- ledger.Index:
		return nil
	default:
		return errors.InternalError.With("channel is full")
	}
}

func (c *Collector) willCommitBlock(e execute.WillCommitBlock) error {
	s := new(messaging.BlockSummary)
	s.Partition = c.partition
	s.Index = e.Block.Params().Index

	select {
	case c.latest <- s:
		// Ok
	default:
		return errors.InternalError.With("already processing another block")
	}

	// Get the index of the previous block
	select {
	case s.PreviousBlock = <-c.previous:
	default:
		return errors.InternalError.With("no previous block")
	}

	err := e.Block.WalkChanges(func(r record.TerminalRecord) error {
		v, _, err := r.GetValue()
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
		b, err := v.MarshalBinary()
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
		s.RecordUpdates = append(s.RecordUpdates, &messaging.RecordUpdate{Key: r.Key(), Value: b})
		return nil
	})
	return errors.UnknownError.Wrap(err)
}

func (c *Collector) didCommitBlock(e events.DidCommitBlock) error {
	if e.Init {
		return nil
	}

	var s *messaging.BlockSummary
	select {
	case s = <-c.latest:
		// Ok
	default:
		return errors.InternalError.With("not processing a block")
	}
	if s.Index != e.Index {
		return errors.InternalError.WithFormat("block index conflict (%d != %d)", s.Index, e.Index)
	}

	batch := c.db.Begin(false)
	defer batch.Discard()

	s.StateTreeHash = *(*[32]byte)(batch.BptRoot())

	seen := map[[32]byte]bool{}
	for _, r := range s.RecordUpdates {
		if len(r.Key) < 2 {
			continue
		}
		typ, ok1 := r.Key[0].(string)
		url, ok2 := r.Key[1].(*url.URL)
		if !ok1 || !ok2 || typ != "Account" {
			continue
		}
		if seen[url.AccountID32()] {
			continue
		}
		seen[url.AccountID32()] = true

		hash, err := batch.Account(url).Hash()
		if err != nil {
			return errors.InternalError.WithFormat("get %v hash: %w", url, err)
		}
		s.StateTreeUpdates = append(s.StateTreeUpdates, &messaging.StateTreeUpdate{Key: r.Key[:2], Hash: hash})
	}

	s.Sort()

	err := c.events.Publish(DidCollectBlock{Summary: s})
	return errors.UnknownError.Wrap(err)
}
