package api

import (
	"context"
	"runtime/debug"
	"sync"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
)

type EventService struct {
	logger      logging.OptionalLogger
	db          database.Beginner
	partitionId string
	partition   config.NetworkUrl

	blockMu    *sync.RWMutex
	blockReady chan struct{}
	lastBlock  *api.BlockEvent
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
	s.blockMu = new(sync.RWMutex)
	s.blockReady = make(chan struct{})
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

	e.Entries = make([]*api.BlockEntry, len(ledger.Entries))
	for i, le := range ledger.Entries {
		entry := new(api.BlockEntry)
		entry.Account = le.Account
		entry.Chain = le.Chain
		entry.Index = le.Index
		e.Entries[i] = entry

		chain, err := batch.Account(le.Account).ChainByName(le.Chain)
		if err != nil {
			s.logger.Error("Loading chain for block entry", "error", err, "block", e.Index, "account", le.Account, "chain", le.Chain, "index", le.Index)
			continue
		}

		if chain.Type() != managed.ChainTypeTransaction {
			continue
		}

		entry.Value, err = chain.Entry(int64(le.Index))
		if err != nil {
			s.logger.Error("Loading chain entry for block entry", "error", err, "block", e.Index, "account", le.Account, "chain", le.Chain, "index", le.Index)
			continue
		}

		state, err := batch.Transaction(entry.Value).Main().Get()
		if err != nil {
			s.logger.Error("Loading transaction for block entry", "error", err, "block", e.Index, "account", le.Account, "chain", le.Chain, "index", le.Index, "entry.Value", logging.AsHex(entry.Value).Slice(0, 4))
			continue
		}
		if state.Transaction != nil {
			entry.TxID = state.Transaction.ID()
			entry.Transaction = state.Transaction
		}
		if state.Signature != nil {
			entry.TxID = state.Txid
			entry.Signature = state.Signature
		}
	}

	// Notify subscribers
	s.blockMu.Lock()
	defer s.blockMu.Unlock()
	ready := s.blockReady
	s.blockReady = make(chan struct{})
	s.lastBlock = e
	close(ready)
}

func (s *EventService) Subscribe(ctx context.Context) (<-chan api.Event, error) {
	ch := make(chan api.Event, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Error("Subscription loop panicked", "error", r, "stack", debug.Stack())
			}
		}()
		defer close(ch)

		s.blockMu.RLock()
		ready := s.blockReady // Get the initial broadcast channel
		last := s.lastBlock   // Get the last block
		s.blockMu.RUnlock()

		// Send the last block. That way, if the client calls QueryRecord for a
		// transaction, doesn't find it, then calls Subscribe, they will
		// certainly receive the block with their transaction unless there's a
		// delay on the order of 1 second between the calls.
		if last != nil {
			ch <- last
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

			s.blockMu.RLock()
			ready = s.blockReady // Get the next broadcast channel
			last = s.lastBlock   // Get the block
			s.blockMu.RUnlock()

			// Send the block
			ch <- last
		}
	}()

	return ch, nil
}
