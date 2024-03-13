// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bsn

import (
	"crypto/ed25519"
	"io"
	"strings"

	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var executors []func(ExecutorOptions) (messaging.MessageType, MessageExecutor)

type Executor struct {
	executors   map[messaging.MessageType]MessageExecutor
	logger      logging.OptionalLogger
	store       keyvalue.Beginner
	eventBus    *events.Bus
	partitionID string
}

type ExecutorOptions struct {
	PartitionID string
	Logger      log.Logger
	Store       keyvalue.Beginner
	EventBus    *events.Bus
}

func NewExecutor(opts ExecutorOptions) (*Executor, error) {
	x := new(Executor)
	x.logger.Set(opts.Logger, "module", "executor")
	x.partitionID = opts.PartitionID
	x.store = opts.Store
	x.eventBus = opts.EventBus
	x.executors = newExecutorMap(opts, executors)

	// Load globals if the database has been initialized
	batch := NewChangeSet(x.store, x.logger)
	defer batch.Discard()
	part := batch.Partition(protocol.Directory)

	var ledger *protocol.SystemLedger
	u := protocol.DnUrl().JoinPath(protocol.Ledger)
	err := part.Account(u).Main().GetAs(&ledger)
	switch {
	case err == nil:
		// Database has been initialized, load globals
		_, err = x.loadGlobals(protocol.Directory, batch, nil, true)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}

	case errors.Is(err, storage.ErrNotFound):
		// Database is uninitialized

	default:
		return nil, errors.UnknownError.WithFormat("load ledger: %w", err)
	}
	return x, nil
}

var _ execute.Executor = (*Executor)(nil)

func (*Executor) EnableTimers()                        {}
func (*Executor) StoreBlockTimers(ds *logging.DataSet) {}

func (x *Executor) LastBlock() (*execute.BlockParams, [32]byte, error) {
	batch := NewChangeSet(x.store, x.logger)
	defer batch.Discard()
	last, err := batch.LastBlock().Get()
	if err != nil {
		return nil, [32]byte{}, errors.UnknownError.WithFormat("load last block info: %w", err)
	}
	p := new(execute.BlockParams)
	p.Index = last.Index
	p.Time = last.Time
	return p, [32]byte{}, nil
}

func LoadSnapshot(file ioutil2.SectionReader, store keyvalue.Beginner, logger log.Logger) error {
	header, rd, err := snapshot.Open(file)
	if err != nil {
		return errors.UnknownError.WithFormat("open snapshot: %w", err)
	}
	if header.Version != snapshot.Version1 {
		return errors.BadRequest.WithFormat("expected version %d, got %d", snapshot.Version1, header.Version)
	}

	// Initialize the database
	batch := NewChangeSet(store, logger)
	defer batch.Discard()
	err = batch.LastBlock().Put(&LastBlock{
		Index: header.Height,
		Time:  header.Timestamp,
	})
	if err != nil {
		return errors.UnknownError.WithFormat("store last block info: %w", err)
	}
	err = batch.Commit()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Restore all the partition's snapshots
	var i int
	for {
		s, err := rd.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return errors.UnknownError.Wrap(err)
		}
		if s.Type() != snapshot.SectionTypeSnapshot {
			continue // Ignore non-snapshot sections
		}
		rd, err := s.Open()
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}

		if i >= len(header.PartitionSnapshotIDs) {
			return errors.BadRequest.With("invalid snapshot: number of sub-snapshots does not match the header")
		}

		id := strings.ToLower(header.PartitionSnapshotIDs[i])
		pb := &partitionBeginner{logger, store, id}
		i++

		err = snapshot.FullRestore(pb, rd, logger, config.NetworkUrl{
			URL: protocol.PartitionUrl(id),
		})
		if err != nil {
			return errors.UnknownError.WithFormat("restore %s: %w", id, err)
		}
	}
	if i < len(header.PartitionSnapshotIDs) {
		return errors.BadRequest.With("invalid snapshot: number of sub-snapshots does not match the header")
	}

	return nil
}

func (x *Executor) Init(validators []*execute.ValidatorUpdate) (additional []*execute.ValidatorUpdate, err error) {
	// Load and publish globals
	batch := NewChangeSet(x.store, x.logger)
	defer batch.Discard()
	g, err := x.loadGlobals(protocol.Directory, batch, nil, true)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Verify the initial keys are ED25519 and build a map
	initValMap := map[[32]byte]bool{}
	for _, val := range validators {
		if val.Type != protocol.SignatureTypeED25519 {
			return nil, errors.BadRequest.WithFormat("validator key type %T is not supported", val.Type)
		}
		if len(val.PublicKey) != ed25519.PublicKeySize {
			return nil, errors.BadRequest.WithFormat("invalid ED25519 key: want length %d, got %d", ed25519.PublicKeySize, len(val.PublicKey))
		}
		initValMap[*(*[32]byte)(val.PublicKey)] = true
	}

	// Capture any validators missing from the initial set
	for _, val := range g.Network.Validators {
		if !val.IsActiveOn(x.partitionID) {
			continue
		}

		if initValMap[*(*[32]byte)(val.PublicKey)] {
			delete(initValMap, *(*[32]byte)(val.PublicKey))
		} else {
			additional = append(additional, &execute.ValidatorUpdate{
				Type:      protocol.SignatureTypeED25519,
				PublicKey: val.PublicKey,
				Power:     1,
			})
		}
	}

	// Verify no additional validators were introduced
	if len(initValMap) > 0 {
		return nil, errors.BadRequest.WithFormat("InitChain request includes %d validator(s) not present in genesis", len(initValMap))
	}

	return additional, nil
}

type partitionBeginner struct {
	logger    log.Logger
	store     keyvalue.Beginner
	partition string
}

func (p *partitionBeginner) SetObserver(observer database.Observer) {}

func (p *partitionBeginner) Begin(writable bool) *database.Batch {
	s := p.store.Begin(record.NewKey(p.partition+"·"), true)
	b := database.NewBatch(p.partition, s, writable, p.logger)
	b.SetObserver(execute.NewDatabaseObserver())
	return b
}

func (p *partitionBeginner) View(fn func(*database.Batch) error) error {
	b := p.Begin(false)
	defer b.Discard()
	return fn(b)
}

func (p *partitionBeginner) Update(fn func(*database.Batch) error) error {
	b := p.Begin(true)
	defer b.Discard()
	err := fn(b)
	if err != nil {
		return err
	}
	return b.Commit()
}

func (x *Executor) loadGlobals(partition string, batch *ChangeSet, old *core.GlobalValues, publish bool) (*core.GlobalValues, error) {
	// Load from the database
	g := new(core.GlobalValues)
	err := g.Load(protocol.PartitionUrl(partition), func(account *url.URL, target interface{}) error {
		return batch.Partition(partition).View(func(batch *database.Batch) error {
			return batch.Account(account).Main().GetAs(target)
		})
	})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load %s globals: %w", partition, err)
	}

	// Should publish an update?
	if !publish {
		return g, nil
	}
	if old != nil && old.Equal(g) {
		return g, nil
	}

	// Publish an update
	err = x.eventBus.Publish(events.WillChangeGlobals{Old: old, New: g})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("publish globals update: %w", err)
	}

	return g, nil
}
