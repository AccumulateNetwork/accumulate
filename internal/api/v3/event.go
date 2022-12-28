// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"runtime/debug"
	"sync"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type EventService struct {
	logger      logging.OptionalLogger
	db          database.Beginner
	partitionId string
	partition   config.NetworkUrl

	eventMu     *sync.RWMutex
	eventReady  chan struct{}
	lastEvent   api.Event
	lastGlobals *api.GlobalsEvent
	lastBlock   *api.BlockEvent
}

var _ api.EventService = (*EventService)(nil)

type EventServiceParams struct {
	Logger    log.Logger
	Database  database.Beginner
	Partition string
	EventBus  *events.Bus
}

func NewEventService(params EventServiceParams) *EventService {
	s := new(EventService)
	s.db = params.Database
	s.logger.Set(params.Logger)
	s.partitionId = params.Partition
	s.partition.URL = protocol.PartitionUrl(params.Partition)
	s.eventMu = new(sync.RWMutex)
	s.eventReady = make(chan struct{})
	events.SubscribeSync(params.EventBus, s.willChangeGlobals)
	events.SubscribeSync(params.EventBus, s.didCommitBlock)
	return s
}

func (s *EventService) didCommitBlock(e events.DidCommitBlock) error {
	event := new(api.BlockEvent)
	event.Partition = s.partitionId
	event.Index = e.Index
	event.Time = e.Time
	event.Major = e.Major

	// Start a batch (synchronously)
	batch := s.db.Begin(false)

	// Process the event (asynchronously)
	go s.loadBlockInfo(batch, event)
	return nil
}

func (s *EventService) loadBlockInfo(batch *database.Batch, e *api.BlockEvent) {
	defer batch.Discard()

	var ledger *protocol.BlockLedger
	err := batch.Account(s.partition.BlockLedger(e.Index)).Main().GetAs(&ledger)
	if err != nil {
		s.logger.Error("Loading block ledger failed", "error", err, "block", e.Index, "url", s.partition.BlockLedger(e.Index))
		return
	}

	e.Entries = make([]*api.ChainEntryRecord[api.Record], len(ledger.Entries))
	for i, le := range ledger.Entries {
		e.Entries[i], err = loadBlockEntry(batch, le)
		if err != nil {
			s.logger.Error("Loading block entry", "error", err, "block", e.Index, "account", le.Account, "chain", le.Chain, "index", le.Index)
			continue
		}
	}

	// Notify subscribers
	s.eventMu.Lock()
	defer s.eventMu.Unlock()
	ready := s.eventReady
	s.eventReady = make(chan struct{})
	s.lastBlock = e
	s.lastEvent = e
	close(ready)
}

func (s *EventService) willChangeGlobals(e events.WillChangeGlobals) error {
	e2 := new(api.GlobalsEvent)
	e2.Old = e.Old
	e2.New = e.New

	go func() {
		// Notify subscribers
		s.eventMu.Lock()
		defer s.eventMu.Unlock()
		ready := s.eventReady
		s.eventReady = make(chan struct{})
		s.lastGlobals = e2
		s.lastEvent = e2
		close(ready)
	}()
	return nil
}

func (s *EventService) Subscribe(ctx context.Context, opts api.SubscribeOptions) (<-chan api.Event, error) {
	ch := make(chan api.Event, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Error("Subscription loop panicked", "error", r, "stack", debug.Stack())
			}
		}()
		defer close(ch)

		s.eventMu.RLock()
		ready := s.eventReady        // Get the initial broadcast channel
		lastBlock := s.lastBlock     // Get the last block
		lastGlobals := s.lastGlobals // Get the last globals
		s.eventMu.RUnlock()

		// Send the last block. That way, if the client calls QueryRecord for a
		// transaction, doesn't find it, then calls Subscribe, they will
		// certainly receive the block with their transaction unless there's a
		// delay on the order of 1 second between the calls.
		if lastBlock != nil {
			ch <- lastBlock
		}
		if lastGlobals != nil {
			ch <- lastGlobals
		}

		for {
			// Wait for the next block
			select {
			case <-ready:
				// Got it
			case <-ctx.Done():
				// Cancelled
				return
			}

			s.eventMu.RLock()
			ready = s.eventReady     // Get the next broadcast channel
			lastEvent := s.lastEvent // Get the block
			s.eventMu.RUnlock()

			// Send the block
			ch <- lastEvent
		}
	}()

	return ch, nil
}
