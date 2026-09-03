// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"runtime/debug"
	"sync"
	"sync/atomic"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
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

	blocks      chan *api.BlockEvent // committed blocks waiting for the loader
	subscribers atomic.Int32         // open Subscribe streams
}

var _ api.EventService = (*EventService)(nil)

type EventServiceParams struct {
	Logger    logging.Logger
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
	s.blocks = make(chan *api.BlockEvent, blockQueueDepth)
	go s.loadBlocks()
	events.SubscribeSync(params.EventBus, s.willChangeGlobals)
	events.SubscribeSync(params.EventBus, s.didCommitBlock)
	return s
}

func (s *EventService) Type() api.ServiceType { return api.ServiceTypeEvent }

// blockQueueDepth bounds the block events waiting to be loaded. Loading a
// block's entries is API work that must never hold the executor's memory:
// the loader runs one at a time, opens its database view only while it
// loads, and a queue that fills drops the oldest event with a warning
// rather than growing (DIFFERENCES D5, PLAN S2).
const blockQueueDepth = 64

func (s *EventService) didCommitBlock(e events.DidCommitBlock) error {
	event := new(api.BlockEvent)
	event.Partition = s.partitionId
	event.Index = e.Index
	event.Time = e.Time
	event.Major = e.Major

	// Hand the event to the loader without opening a batch here: a batch
	// begun at commit time pins this version for as long as the loader
	// takes, and the loader's time grows with the block. Every version so
	// pinned is a commit the store must keep in memory.
	for {
		select {
		case s.blocks <- event:
			return nil
		default:
		}
		select {
		case dropped := <-s.blocks:
			s.logger.Error("Block event queue full, dropping oldest", "dropped", dropped.Index, "queued", e.Index)
		default:
		}
	}
}

// loadBlocks is the single loader. It publishes every block event, and
// loads the block's entries only when someone is subscribed to receive them.
func (s *EventService) loadBlocks() {
	for e := range s.blocks {
		if s.subscribers.Load() > 0 {
			s.loadBlockInfo(e)
		}
		s.publish(e)
	}
}

// loadBlockInfo fills in the block's entries, holding a database view only
// for the duration of the load.
func (s *EventService) loadBlockInfo(e *api.BlockEvent) {
	batch := s.db.Begin(false)
	defer batch.Discard()

	_, entries, err := indexing.LoadBlockLedger(batch.Account(s.partition.Ledger()), e.Index)
	if err != nil {
		s.logger.Error("Loading block ledger failed", "error", err, "block", e.Index, "url", s.partition.BlockLedger(e.Index))
		return
	}

	e.Entries = make([]*api.ChainEntryRecord[api.Record], len(entries))
	for i, le := range entries {
		var err error
		e.Entries[i], err = loadBlockEntry(batch, le)
		if err != nil {
			s.logger.Error("Loading block entry", "error", err, "block", e.Index, "account", le.Account, "chain", le.Chain, "index", le.Index)
			continue
		}
	}
}

// publish makes e the last block and wakes every subscriber.
func (s *EventService) publish(e *api.BlockEvent) {
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

		s.subscribers.Add(1)
		defer s.subscribers.Add(-1)

		s.eventMu.RLock()
		ready := s.eventReady        // Get the initial broadcast channel
		lastBlock := s.lastBlock     // Get the last block
		lastGlobals := s.lastGlobals // Get the last globals
		s.eventMu.RUnlock()

		// Send the last block. That way, if the client calls QueryRecord for a
		// transaction, doesn't find it, then calls Subscribe, they will
		// certainly receive the block with their transaction unless there's a
		// delay on the order of 1 second between the calls. Its entries were
		// not loaded if nobody was subscribed when it was published; load
		// them now, for this subscriber.
		if lastBlock != nil {
			if lastBlock.Entries == nil {
				lastBlock = copyBlockEvent(lastBlock)
				s.loadBlockInfo(lastBlock)
			}
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

func copyBlockEvent(e *api.BlockEvent) *api.BlockEvent {
	c := *e
	return &c
}
