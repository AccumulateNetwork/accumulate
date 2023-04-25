// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bsn

// GENERATED BY go run ./tools/cmd/gen-model. DO NOT EDIT.

//lint:file-ignore S1008,U1000 generated code

import (
	"encoding/hex"
	"strconv"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type ChangeSet struct {
	logger  logging.OptionalLogger
	store   record.Store
	kvstore storage.KeyValueTxn
	parent  *ChangeSet

	lastBlock record.Value[*LastBlock]
	summary   map[summaryKey]*Summary
	pending   map[pendingKey]*Pending
	partition map[partitionKey]*database.Batch
}

type summaryKey struct {
	Hash [32]byte
}

func keyForSummary(hash [32]byte) summaryKey {
	return summaryKey{hash}
}

type pendingKey struct {
	Partition string
}

func keyForPending(partition string) pendingKey {
	return pendingKey{partition}
}

type partitionKey struct {
	ID string
}

func keyForPartition(id string) partitionKey {
	return partitionKey{id}
}

func (c *ChangeSet) LastBlock() record.Value[*LastBlock] {
	return record.FieldGetOrCreate(&c.lastBlock, func() record.Value[*LastBlock] {
		return record.NewValue(c.logger.L, c.store, (*record.Key)(nil).Append("LastBlock"), "last block", false, record.Struct[LastBlock]())
	})
}

func (c *ChangeSet) Summary(hash [32]byte) *Summary {
	return record.FieldGetOrCreateMap(&c.summary, keyForSummary(hash), func() *Summary {
		v := new(Summary)
		v.logger = c.logger
		v.store = c.store
		v.key = (*record.Key)(nil).Append("Summary", hash)
		v.parent = c
		v.label = "summary" + " " + hex.EncodeToString(hash[:])
		return v
	})
}

func (c *ChangeSet) Pending(partition string) *Pending {
	return record.FieldGetOrCreateMap(&c.pending, keyForPending(partition), func() *Pending {
		v := new(Pending)
		v.logger = c.logger
		v.store = c.store
		v.key = (*record.Key)(nil).Append("Pending", partition)
		v.parent = c
		v.label = "pending" + " " + partition
		return v
	})
}

func (c *ChangeSet) Resolve(key *record.Key) (record.Record, *record.Key, error) {
	if key.Len() == 0 {
		return nil, nil, errors.InternalError.With("bad key for change set")
	}

	switch key.Get(0) {
	case "LastBlock":
		return c.LastBlock(), key.SliceI(1), nil
	case "Summary":
		if key.Len() < 2 {
			return nil, nil, errors.InternalError.With("bad key for change set")
		}
		hash, okHash := key.Get(1).([32]byte)
		if !okHash {
			return nil, nil, errors.InternalError.With("bad key for change set")
		}
		v := c.Summary(hash)
		return v, key.SliceI(2), nil
	case "Pending":
		if key.Len() < 2 {
			return nil, nil, errors.InternalError.With("bad key for change set")
		}
		partition, okPartition := key.Get(1).(string)
		if !okPartition {
			return nil, nil, errors.InternalError.With("bad key for change set")
		}
		v := c.Pending(partition)
		return v, key.SliceI(2), nil
	case "Partition":
		if key.Len() < 2 {
			return nil, nil, errors.InternalError.With("bad key for change set")
		}
		id, okID := key.Get(1).(string)
		if !okID {
			return nil, nil, errors.InternalError.With("bad key for change set")
		}
		v := c.Partition(id)
		return v, key.SliceI(2), nil
	default:
		return nil, nil, errors.InternalError.With("bad key for change set")
	}
}

func (c *ChangeSet) IsDirty() bool {
	if c == nil {
		return false
	}

	if record.FieldIsDirty(c.lastBlock) {
		return true
	}
	for _, v := range c.summary {
		if v.IsDirty() {
			return true
		}
	}
	for _, v := range c.pending {
		if v.IsDirty() {
			return true
		}
	}
	for _, v := range c.partition {
		if v.IsDirty() {
			return true
		}
	}

	return false
}

func (c *ChangeSet) WalkChanges(fn record.WalkFunc) error {
	if c == nil {
		return nil
	}

	var err error
	record.FieldWalkChanges(&err, c.lastBlock, fn)
	for _, v := range c.summary {
		record.FieldWalkChanges(&err, v, fn)
	}
	for _, v := range c.pending {
		record.FieldWalkChanges(&err, v, fn)
	}
	for _, v := range c.partition {
		record.FieldWalkChanges(&err, v, fn)
	}
	return err
}

func (c *ChangeSet) baseCommit() error {
	if c == nil {
		return nil
	}

	var err error
	record.FieldCommit(&err, c.lastBlock)
	for _, v := range c.summary {
		record.FieldCommit(&err, v)
	}
	for _, v := range c.pending {
		record.FieldCommit(&err, v)
	}
	for _, v := range c.partition {
		record.FieldCommit(&err, v)
	}

	return err
}

type Summary struct {
	logger logging.OptionalLogger
	store  record.Store
	key    *record.Key
	label  string
	parent *ChangeSet

	main       record.Value[*messaging.BlockSummary]
	signatures record.Set[protocol.KeySignature]
}

func (c *Summary) Main() record.Value[*messaging.BlockSummary] {
	return record.FieldGetOrCreate(&c.main, func() record.Value[*messaging.BlockSummary] {
		return record.NewValue(c.logger.L, c.store, c.key.Append("Main"), c.label+" "+"main", false, record.Struct[messaging.BlockSummary]())
	})
}

func (c *Summary) Signatures() record.Set[protocol.KeySignature] {
	return record.FieldGetOrCreate(&c.signatures, func() record.Set[protocol.KeySignature] {
		return record.NewSet(c.logger.L, c.store, c.key.Append("Signatures"), c.label+" "+"signatures", record.Union(protocol.UnmarshalKeySignature), compareSignatures)
	})
}

func (c *Summary) Resolve(key *record.Key) (record.Record, *record.Key, error) {
	if key.Len() == 0 {
		return nil, nil, errors.InternalError.With("bad key for summary")
	}

	switch key.Get(0) {
	case "Main":
		return c.Main(), key.SliceI(1), nil
	case "Signatures":
		return c.Signatures(), key.SliceI(1), nil
	default:
		return nil, nil, errors.InternalError.With("bad key for summary")
	}
}

func (c *Summary) IsDirty() bool {
	if c == nil {
		return false
	}

	if record.FieldIsDirty(c.main) {
		return true
	}
	if record.FieldIsDirty(c.signatures) {
		return true
	}

	return false
}

func (c *Summary) WalkChanges(fn record.WalkFunc) error {
	if c == nil {
		return nil
	}

	var err error
	record.FieldWalkChanges(&err, c.main, fn)
	record.FieldWalkChanges(&err, c.signatures, fn)
	return err
}

func (c *Summary) Commit() error {
	if c == nil {
		return nil
	}

	var err error
	record.FieldCommit(&err, c.main)
	record.FieldCommit(&err, c.signatures)

	return err
}

type Pending struct {
	logger logging.OptionalLogger
	store  record.Store
	key    *record.Key
	label  string
	parent *ChangeSet

	onBlock map[pendingOnBlockKey]record.Value[[32]byte]
}

type pendingOnBlockKey struct {
	Index uint64
}

func keyForPendingOnBlock(index uint64) pendingOnBlockKey {
	return pendingOnBlockKey{index}
}

func (c *Pending) OnBlock(index uint64) record.Value[[32]byte] {
	return record.FieldGetOrCreateMap(&c.onBlock, keyForPendingOnBlock(index), func() record.Value[[32]byte] {
		return record.NewValue(c.logger.L, c.store, c.key.Append("OnBlock", index), c.label+" "+"on block"+" "+strconv.FormatUint(index, 10), false, record.Wrapped(record.HashWrapper))
	})
}

func (c *Pending) Resolve(key *record.Key) (record.Record, *record.Key, error) {
	if key.Len() == 0 {
		return nil, nil, errors.InternalError.With("bad key for pending")
	}

	switch key.Get(0) {
	case "OnBlock":
		if key.Len() < 2 {
			return nil, nil, errors.InternalError.With("bad key for pending")
		}
		index, okIndex := key.Get(1).(uint64)
		if !okIndex {
			return nil, nil, errors.InternalError.With("bad key for pending")
		}
		v := c.OnBlock(index)
		return v, key.SliceI(2), nil
	default:
		return nil, nil, errors.InternalError.With("bad key for pending")
	}
}

func (c *Pending) IsDirty() bool {
	if c == nil {
		return false
	}

	for _, v := range c.onBlock {
		if v.IsDirty() {
			return true
		}
	}

	return false
}

func (c *Pending) WalkChanges(fn record.WalkFunc) error {
	if c == nil {
		return nil
	}

	var err error
	for _, v := range c.onBlock {
		record.FieldWalkChanges(&err, v, fn)
	}
	return err
}

func (c *Pending) Commit() error {
	if c == nil {
		return nil
	}

	var err error
	for _, v := range c.onBlock {
		record.FieldCommit(&err, v)
	}

	return err
}
