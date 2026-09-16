// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"sync"
	"sync/atomic"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/synthcache"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Sequencer struct {
	logger      logging.OptionalLogger
	db          database.Viewer
	cache       *synthcache.Cache
	partitionID string
	partition   config.NetworkUrl
	valKey      []byte
	globals     atomic.Value

	// The pinned sync-epoch snapshot (#4058)
	snapMu sync.Mutex
	snap   *pinnedSnapshot

	// The latest provable state view, captured synchronously at commit
	viewMu        sync.Mutex
	provable      *database.Batch
	provableBlock uint64

	// Recent block -> (consensus round, committee epoch), from DidCommitBlock
	// events. A fast-syncing node needs its epoch block's round and epoch to
	// rejoin consensus; nothing else records the mapping (#4058).
	commitMu     sync.Mutex
	commitRounds map[uint64]blockCommit
	commitOldest uint64
}

var _ private.Sequencer = (*Sequencer)(nil)

type blockCommit struct {
	round uint64
	epoch uint64
}

// commitRoundRetention bounds how many block->round mappings are kept.
const commitRoundRetention = 1 << 14

type SequencerParams struct {
	Logger       logging.Logger
	Database     database.Viewer
	EventBus     *events.Bus
	Globals      *core.GlobalValues
	Partition    string
	ValidatorKey []byte

	// Cache is the producer's synthetic/anchor cache the executor fills.
	// With it, every answer is built from the cache and a miss is refused
	// and counted (healing spec, "The cache"). Without it — only the v1
	// simulator, which has no cache — answers are read from the store.
	Cache *synthcache.Cache
}

func NewSequencer(params SequencerParams) *Sequencer {
	// A sequencer answers from the producer cache and nothing else (healing
	// spec, "The answer"): no chain walk, no receipt built from the store, no
	// database read. There is no second implementation to fall back to, so a
	// missing cache is a configuration error, not a slower path.
	if params.Cache == nil {
		panic("sequencer requires a producer cache")
	}

	s := new(Sequencer)
	s.logger.L = params.Logger
	s.db = params.Database
	s.cache = params.Cache
	s.partitionID = params.Partition
	s.partition.URL = protocol.PartitionUrl(params.Partition)
	s.valKey = params.ValidatorKey
	s.globals.Store(params.Globals)
	events.SubscribeSync(params.EventBus, func(e events.WillChangeGlobals) error {
		s.globals.Store(e.New.Copy())
		return nil
	})
	s.commitRounds = map[uint64]blockCommit{}
	events.SubscribeSync(params.EventBus, func(e events.DidCommitBlock) error {
		if e.Round == 0 {
			return nil // CometBFT -- no rounds
		}
		s.captureProvableView(e.Index)
		s.commitMu.Lock()
		defer s.commitMu.Unlock()
		s.commitRounds[e.Index] = blockCommit{round: e.Round, epoch: e.Epoch}
		if s.commitOldest == 0 {
			s.commitOldest = e.Index
		}
		for e.Index-s.commitOldest > commitRoundRetention {
			delete(s.commitRounds, s.commitOldest)
			s.commitOldest++
		}
		return nil
	})
	return s
}

func (s *Sequencer) Type() api.ServiceType { return private.ServiceTypeSequencer }

func (s *Sequencer) Sequence(ctx context.Context, src, dst *url.URL, num uint64, _ private.SequenceOptions) (*api.MessageRecord[messaging.Message], error) {
	// Admission control — proof building is the most expensive query this
	// node serves, and heal storms poll it hardest exactly when the node is
	// slowest (#4164). See gate.go.
	if err := sequenceGate.enter(ctx); err != nil {
		return nil, err
	}
	defer sequenceGate.exit()

	if src == nil {
		return nil, errors.BadRequest.With("missing source")
	}
	if dst == nil {
		return nil, errors.BadRequest.With("missing destination")
	}
	if num == 0 {
		return nil, errors.BadRequest.With("missing sequence number")
	}
	if !s.partition.URL.ParentOf(src) {
		return nil, errors.BadRequest.WithFormat("requested source is %s but this partition is %s", src.RootIdentity(), s.partitionID)
	}

	globals := s.globals.Load().(*core.GlobalValues)
	if globals == nil {
		return nil, errors.NotReady
	}

	// Starting a batch would not be safe if the ABCI were updated to commit in
	// the middle of a block

	switch {
	case s.partition.Synthetic().Equal(src):
		return s.getSynthFromCache(globals, dst, num)
	case s.partition.AnchorPool().Equal(src):
		return s.getAnchorFromCache(globals, dst, num)
	}

	return nil, errors.BadRequest.WithFormat("invalid source: %s", src)
}

// SequenceRange implements [private.SequenceRanger.SequenceRange]: it serves
// synthetic messages start through end (inclusive, 1-based) for the given
// destination, with a single collection proof (#4048) covering the whole
// range, set as SourceReceiptList on the last record.
func (s *Sequencer) SequenceRange(ctx context.Context, src, dst *url.URL, start, end uint64, opts private.SequenceOptions) ([]*api.MessageRecord[messaging.Message], error) {
	// Admission control — see gate.go and the note on Sequence.
	if err := sequenceGate.enter(ctx); err != nil {
		return nil, err
	}
	defer sequenceGate.exit()

	if src == nil {
		return nil, errors.BadRequest.With("missing source")
	}
	if dst == nil {
		return nil, errors.BadRequest.With("missing destination")
	}
	if start == 0 {
		return nil, errors.BadRequest.With("missing start")
	}
	if end < start {
		return nil, errors.BadRequest.WithFormat("invalid range [%d, %d]", start, end)
	}
	if end-start+1 > protocol.MaxReceiptListElements {
		return nil, errors.BadRequest.WithFormat("range [%d, %d] exceeds %d elements", start, end, protocol.MaxReceiptListElements)
	}
	if !s.partition.URL.ParentOf(src) {
		return nil, errors.BadRequest.WithFormat("requested source is %s but this partition is %s", src.RootIdentity(), s.partitionID)
	}

	globals := s.globals.Load().(*core.GlobalValues)
	if globals == nil {
		return nil, errors.NotReady
	}

	switch {
	case s.partition.Synthetic().Equal(src):
		return s.getSynthRangeFromCache(globals, dst, start, end, opts)
	case s.partition.AnchorPool().Equal(src):
		return s.getAnchorRangeFromCache(globals, dst, start, end)
	default:
		return nil, errors.BadRequest.With("only synthetic and anchor sequence ranges are supported")
	}
}

// commitRoundFor returns the consensus round and committee epoch that
// committed the given block, if known.
func (s *Sequencer) commitRoundFor(block uint64) (blockCommit, bool) {
	s.commitMu.Lock()
	defer s.commitMu.Unlock()
	c, ok := s.commitRounds[block]
	return c, ok
}

// getReceiptForChainEntry gets a receipt from an entry to the first anchor
// after that entry.
func (s *Sequencer) getReceiptForChainEntry(chain *database.Chain2, index uint64) (*merkle.Receipt, *protocol.IndexEntry, error) {
	// Load the index chain
	indexChain, err := chain.Index().Get()
	if err != nil {
		return nil, nil, errors.UnknownError.WithFormat("load %s index chain: %w", chain.Name(), err)
	}
	if indexChain.Height() == 0 {
		return nil, nil, errors.Conflict.WithFormat("%s index chain is empty", chain.Name())
	}

	// Locate the index entry for the given entry
	_, entry, err := indexing.SearchIndexChain(indexChain, uint64(indexChain.Height()-1), indexing.MatchAfter, indexing.SearchIndexChainBySource(index))
	if err != nil {
		return nil, nil, errors.UnknownError.WithFormat("locate index entry for %s chain entry %d: %w", chain.Name(), index, err)
	}

	// Load the chain
	c, err := chain.Get()
	if err != nil {
		return nil, nil, errors.UnknownError.WithFormat("load %s chain: %w", chain.Name(), err)
	}

	// Get a receipt
	receipt, err := c.Receipt(int64(index), int64(entry.Source))
	if err != nil {
		return nil, nil, errors.UnknownError.WithFormat("get %s chain receipt from %d to %d: %w", chain.Name(), index, entry.Source, err)
	}

	return receipt, entry, nil
}

// getRootReceipt gets a root chain receipt.
func (s *Sequencer) getRootReceipt(batch *database.Batch, from, to uint64) (*merkle.Receipt, error) {
	// Load the root chain
	root, err := batch.Account(s.partition.Ledger()).RootChain().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load root chain: %w", err)
	}

	// Get a receipt from the entry to the block's anchor
	receipt, err := root.Receipt(int64(from), int64(to))
	if err != nil {
		return nil, errors.UnknownError.WithFormat("get root chain receipt from %d to %d: %w", from, to, err)
	}
	return receipt, nil
}
