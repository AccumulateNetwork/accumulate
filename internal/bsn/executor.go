// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bsn

import (
	"io"
	"strings"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

var executors []func(ExecutorOptions) (messaging.MessageType, MessageExecutor)

type Executor struct {
	executors map[messaging.MessageType]MessageExecutor
	logger    logging.OptionalLogger
	store     storage.KeyValueStore
}

type ExecutorOptions struct {
	Logger log.Logger
	Store  storage.KeyValueStore
}

func NewExecutor(opts ExecutorOptions) (*Executor, error) {
	x := new(Executor)
	x.logger.Set(opts.Logger, "module", "executor")
	x.store = opts.Store
	x.executors = newExecutorMap(opts, executors)
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

func (x *Executor) Restore(file ioutil2.SectionReader, validators []*execute.ValidatorUpdate) (additional []*execute.ValidatorUpdate, err error) {
	header, rd, err := snapshot.Open(file)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open snapshot: %w", err)
	}
	if header.Version != snapshot.Version1 {
		return nil, errors.BadRequest.WithFormat("expected version %d, got %d", snapshot.Version1, header.Version)
	}

	batch := NewChangeSet(x.store, x.logger)
	defer batch.Discard()
	err = batch.LastBlock().Put(&LastBlock{
		Index: header.Height,
		Time:  header.Timestamp,
	})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store last block info: %w", err)
	}
	err = batch.Commit()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	var i int
	for {
		s, err := rd.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, errors.UnknownError.Wrap(err)
		}
		if s.Type() != snapshot.SectionTypeSnapshot {
			continue // Ignore non-snapshot sections
		}
		rd, err := s.Open()
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}

		if i >= len(header.PartitionSnapshotIDs) {
			return nil, errors.BadRequest.With("invalid snapshot: number of sub-snapshots does not match the header")
		}

		id := strings.ToLower(header.PartitionSnapshotIDs[i])
		pb := &partitionBeginner{x.logger, x.store, id}
		i++

		err = snapshot.Restore(pb, rd, nil)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("restore %s: %w", id, err)
		}
	}
	if i < len(header.PartitionSnapshotIDs) {
		return nil, errors.BadRequest.With("invalid snapshot: number of sub-snapshots does not match the header")
	}

	return nil, nil
}

type partitionBeginner struct {
	logger    log.Logger
	store     storage.KeyValueStore
	partition string
}

func (p *partitionBeginner) SetObserver(observer database.Observer) {}

func (p *partitionBeginner) Begin(writable bool) *database.Batch {
	s := p.store.Begin(writable)
	b := database.NewBatch(p.partition, record.Key{"Partition", p.partition}, s, writable, p.logger)
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
