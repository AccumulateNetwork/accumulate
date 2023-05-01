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
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	record "gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type ChangeSet struct {
	logger  logging.OptionalLogger
	store   record.Store
	kvstore keyvalue.ChangeSet
	parent  *ChangeSet

	lastBlock values.Value[*LastBlock]
	summary   map[summaryKey]*Summary
	pending   map[pendingKey]*Pending
	partition map[partitionKey]*database.Batch
}

func (c *ChangeSet) Key() *record.Key { return nil }

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

func (c *ChangeSet) LastBlock() values.Value[*LastBlock] {
	return values.GetOrCreate(&c.lastBlock, (*ChangeSet).newLastBlock, c)
}

func (c *ChangeSet) newLastBlock() values.Value[*LastBlock] {
	return values.NewValue(c.logger.L, c.store, (*record.Key)(nil).Append("LastBlock"), "last block", false, values.Struct[LastBlock]())
}

func (c *ChangeSet) Summary(hash [32]byte) *Summary {
	return values.GetOrCreateMap1(&c.summary, keyForSummary(hash), (*ChangeSet).newSummary, c, hash)
}

func (c *ChangeSet) newSummary(hash [32]byte) *Summary {
	v := new(Summary)
	v.logger = c.logger
	v.store = c.store
	v.key = (*record.Key)(nil).Append("Summary", hash)
	v.parent = c
	v.label = "summary" + " " + hex.EncodeToString(hash[:])
	return v
}

func (c *ChangeSet) Pending(partition string) *Pending {
	return values.GetOrCreateMap1(&c.pending, keyForPending(partition), (*ChangeSet).newPending, c, partition)
}

func (c *ChangeSet) newPending(partition string) *Pending {
	v := new(Pending)
	v.logger = c.logger
	v.store = c.store
	v.key = (*record.Key)(nil).Append("Pending", partition)
	v.parent = c
	v.label = "pending" + " " + partition
	return v
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

	if values.IsDirty(c.lastBlock) {
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

func (c *ChangeSet) Walk(opts record.WalkOptions, fn record.WalkFunc) error {
	if c == nil {
		return nil
	}

	skip, err := values.WalkComposite(c, opts, fn)
	if skip || err != nil {
		return errors.UnknownError.Wrap(err)
	}
	values.WalkField(&err, c.lastBlock, c.LastBlock, opts, fn)
	for _, v := range c.summary {
		values.Walk(&err, v, opts, fn)
	}
	for _, v := range c.pending {
		values.Walk(&err, v, opts, fn)
	}
	for _, v := range c.partition {
		values.Walk(&err, v, opts, fn)
	}
	return err
}

func (c *ChangeSet) baseCommit() error {
	if c == nil {
		return nil
	}

	var err error
	values.Commit(&err, c.lastBlock)
	for _, v := range c.summary {
		values.Commit(&err, v)
	}
	for _, v := range c.pending {
		values.Commit(&err, v)
	}
	for _, v := range c.partition {
		values.Commit(&err, v)
	}

	return err
}

type Summary struct {
	logger logging.OptionalLogger
	store  record.Store
	key    *record.Key
	label  string
	parent *ChangeSet

	main       values.Value[*messaging.BlockSummary]
	signatures values.Set[protocol.KeySignature]
}

func (c *Summary) Key() *record.Key { return c.key }

func (c *Summary) Main() values.Value[*messaging.BlockSummary] {
	return values.GetOrCreate(&c.main, (*Summary).newMain, c)
}

func (c *Summary) newMain() values.Value[*messaging.BlockSummary] {
	return values.NewValue(c.logger.L, c.store, c.key.Append("Main"), c.label+" "+"main", false, values.Struct[messaging.BlockSummary]())
}

func (c *Summary) Signatures() values.Set[protocol.KeySignature] {
	return values.GetOrCreate(&c.signatures, (*Summary).newSignatures, c)
}

func (c *Summary) newSignatures() values.Set[protocol.KeySignature] {
	return values.NewSet(c.logger.L, c.store, c.key.Append("Signatures"), c.label+" "+"signatures", values.Union(protocol.UnmarshalKeySignature), compareSignatures)
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

	if values.IsDirty(c.main) {
		return true
	}
	if values.IsDirty(c.signatures) {
		return true
	}

	return false
}

func (c *Summary) Walk(opts record.WalkOptions, fn record.WalkFunc) error {
	if c == nil {
		return nil
	}

	skip, err := values.WalkComposite(c, opts, fn)
	if skip || err != nil {
		return errors.UnknownError.Wrap(err)
	}
	values.WalkField(&err, c.main, c.Main, opts, fn)
	values.WalkField(&err, c.signatures, c.Signatures, opts, fn)
	return err
}

func (c *Summary) Commit() error {
	if c == nil {
		return nil
	}

	var err error
	values.Commit(&err, c.main)
	values.Commit(&err, c.signatures)

	return err
}

type Pending struct {
	logger logging.OptionalLogger
	store  record.Store
	key    *record.Key
	label  string
	parent *ChangeSet

	onBlock map[pendingOnBlockKey]values.Value[[32]byte]
}

func (c *Pending) Key() *record.Key { return c.key }

type pendingOnBlockKey struct {
	Index uint64
}

func keyForPendingOnBlock(index uint64) pendingOnBlockKey {
	return pendingOnBlockKey{index}
}

func (c *Pending) OnBlock(index uint64) values.Value[[32]byte] {
	return values.GetOrCreateMap1(&c.onBlock, keyForPendingOnBlock(index), (*Pending).newOnBlock, c, index)
}

func (c *Pending) newOnBlock(index uint64) values.Value[[32]byte] {
	return values.NewValue(c.logger.L, c.store, c.key.Append("OnBlock", index), c.label+" "+"on block"+" "+strconv.FormatUint(index, 10), false, values.Wrapped(values.HashWrapper))
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

func (c *Pending) Walk(opts record.WalkOptions, fn record.WalkFunc) error {
	if c == nil {
		return nil
	}

	skip, err := values.WalkComposite(c, opts, fn)
	if skip || err != nil {
		return errors.UnknownError.Wrap(err)
	}
	for _, v := range c.onBlock {
		values.Walk(&err, v, opts, fn)
	}
	return err
}

func (c *Pending) Commit() error {
	if c == nil {
		return nil
	}

	var err error
	for _, v := range c.onBlock {
		values.Commit(&err, v)
	}

	return err
}
