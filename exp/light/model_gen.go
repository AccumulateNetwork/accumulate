// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package light

// GENERATED BY go run ./tools/cmd/gen-model. DO NOT EDIT.

//lint:file-ignore S1008,U1000 generated code

import (
	"encoding/hex"

	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	record "gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type IndexDB interface {
	record.Record
	Account(url *url.URL) IndexDBAccount
	Partition(url *url.URL) IndexDBPartition
	Transaction(hash [32]byte) IndexDBTransaction
}

type indexDB struct {
	logger logging.OptionalLogger
	store  record.Store
	key    *record.Key
	label  string

	account     map[indexDBAccountKey]*indexDBAccount
	partition   map[indexDBPartitionKey]*indexDBPartition
	transaction map[indexDBTransactionKey]*indexDBTransaction
}

func (c *indexDB) Key() *record.Key { return c.key }

type indexDBAccountKey struct {
	Url [32]byte
}

func keyForIndexDBAccount(url *url.URL) indexDBAccountKey {
	return indexDBAccountKey{values.MapKeyUrl(url)}
}

type indexDBPartitionKey struct {
	Url [32]byte
}

func keyForIndexDBPartition(url *url.URL) indexDBPartitionKey {
	return indexDBPartitionKey{values.MapKeyUrl(url)}
}

type indexDBTransactionKey struct {
	Hash [32]byte
}

func keyForIndexDBTransaction(hash [32]byte) indexDBTransactionKey {
	return indexDBTransactionKey{hash}
}

func (c *indexDB) Account(url *url.URL) IndexDBAccount {
	return values.GetOrCreateMap(&c.account, keyForIndexDBAccount(url), func() *indexDBAccount {
		return c.newAccount(url)
	})
}

func (c *indexDB) newAccount(url *url.URL) *indexDBAccount {
	v := new(indexDBAccount)
	v.logger = c.logger
	v.store = c.store
	v.key = c.key.Append("Account", url)
	v.parent = c
	v.label = c.label + " " + "account" + " " + url.RawString()
	return v
}

func (c *indexDB) Partition(url *url.URL) IndexDBPartition {
	return values.GetOrCreateMap(&c.partition, keyForIndexDBPartition(url), func() *indexDBPartition {
		return c.newPartition(url)
	})
}

func (c *indexDB) newPartition(url *url.URL) *indexDBPartition {
	v := new(indexDBPartition)
	v.logger = c.logger
	v.store = c.store
	v.key = c.key.Append("Partition", url)
	v.parent = c
	v.label = c.label + " " + "partition" + " " + url.RawString()
	return v
}

func (c *indexDB) Transaction(hash [32]byte) IndexDBTransaction {
	return values.GetOrCreateMap(&c.transaction, keyForIndexDBTransaction(hash), func() *indexDBTransaction {
		return c.newTransaction(hash)
	})
}

func (c *indexDB) newTransaction(hash [32]byte) *indexDBTransaction {
	v := new(indexDBTransaction)
	v.logger = c.logger
	v.store = c.store
	v.key = c.key.Append("Transaction", hash)
	v.parent = c
	v.label = c.label + " " + "transaction" + " " + hex.EncodeToString(hash[:])
	return v
}

func (c *indexDB) Resolve(key *record.Key) (record.Record, *record.Key, error) {
	if key.Len() == 0 {
		return nil, nil, errors.InternalError.With("bad key for index db")
	}

	switch key.Get(0) {
	case "Account":
		if key.Len() < 2 {
			return nil, nil, errors.InternalError.With("bad key for index db")
		}
		url, okUrl := key.Get(1).(*url.URL)
		if !okUrl {
			return nil, nil, errors.InternalError.With("bad key for index db")
		}
		v := c.Account(url)
		return v, key.SliceI(2), nil
	case "Partition":
		if key.Len() < 2 {
			return nil, nil, errors.InternalError.With("bad key for index db")
		}
		url, okUrl := key.Get(1).(*url.URL)
		if !okUrl {
			return nil, nil, errors.InternalError.With("bad key for index db")
		}
		v := c.Partition(url)
		return v, key.SliceI(2), nil
	case "Transaction":
		if key.Len() < 2 {
			return nil, nil, errors.InternalError.With("bad key for index db")
		}
		hash, okHash := key.Get(1).([32]byte)
		if !okHash {
			return nil, nil, errors.InternalError.With("bad key for index db")
		}
		v := c.Transaction(hash)
		return v, key.SliceI(2), nil
	default:
		return nil, nil, errors.InternalError.With("bad key for index db")
	}
}

func (c *indexDB) IsDirty() bool {
	if c == nil {
		return false
	}

	for _, v := range c.account {
		if v.IsDirty() {
			return true
		}
	}
	for _, v := range c.partition {
		if v.IsDirty() {
			return true
		}
	}
	for _, v := range c.transaction {
		if v.IsDirty() {
			return true
		}
	}

	return false
}

func (c *indexDB) Walk(opts record.WalkOptions, fn record.WalkFunc) error {
	if c == nil {
		return nil
	}

	skip, err := values.WalkComposite(c, opts, fn)
	if skip || err != nil {
		return errors.UnknownError.Wrap(err)
	}
	for _, v := range c.account {
		values.Walk(&err, v, opts, fn)
	}
	for _, v := range c.partition {
		values.Walk(&err, v, opts, fn)
	}
	for _, v := range c.transaction {
		values.Walk(&err, v, opts, fn)
	}
	return err
}

func (c *indexDB) Commit() error {
	if c == nil {
		return nil
	}

	var err error
	for _, v := range c.account {
		values.Commit(&err, v)
	}
	for _, v := range c.partition {
		values.Commit(&err, v)
	}
	for _, v := range c.transaction {
		values.Commit(&err, v)
	}

	return err
}

type IndexDBAccount interface {
	record.Record
	DidIndexTransactionExecution() values.Set[[32]byte]
	DidLoadTransaction() values.Set[[32]byte]
	Chain(name string) IndexDBAccountChain
}

type indexDBAccount struct {
	logger logging.OptionalLogger
	store  record.Store
	key    *record.Key
	label  string
	parent *indexDB

	didIndexTransactionExecution values.Set[[32]byte]
	didLoadTransaction           values.Set[[32]byte]
	chain                        map[indexDBAccountChainKey]*indexDBAccountChain
}

func (c *indexDBAccount) Key() *record.Key { return c.key }

type indexDBAccountChainKey struct {
	Name string
}

func keyForIndexDBAccountChain(name string) indexDBAccountChainKey {
	return indexDBAccountChainKey{name}
}

func (c *indexDBAccount) DidIndexTransactionExecution() values.Set[[32]byte] {
	return values.GetOrCreate(&c.didIndexTransactionExecution, c.newDidIndexTransactionExecution)
}

func (c *indexDBAccount) newDidIndexTransactionExecution() values.Set[[32]byte] {
	return values.NewSet(c.logger.L, c.store, c.key.Append("DidIndexTransactionExecution"), c.label+" "+"did index transaction execution", values.Wrapped(values.HashWrapper), values.CompareHash)
}

func (c *indexDBAccount) DidLoadTransaction() values.Set[[32]byte] {
	return values.GetOrCreate(&c.didLoadTransaction, c.newDidLoadTransaction)
}

func (c *indexDBAccount) newDidLoadTransaction() values.Set[[32]byte] {
	return values.NewSet(c.logger.L, c.store, c.key.Append("DidLoadTransaction"), c.label+" "+"did load transaction", values.Wrapped(values.HashWrapper), values.CompareHash)
}

func (c *indexDBAccount) Chain(name string) IndexDBAccountChain {
	return values.GetOrCreateMap(&c.chain, keyForIndexDBAccountChain(name), func() *indexDBAccountChain {
		return c.newChain(name)
	})
}

func (c *indexDBAccount) newChain(name string) *indexDBAccountChain {
	v := new(indexDBAccountChain)
	v.logger = c.logger
	v.store = c.store
	v.key = c.key.Append("Chain", name)
	v.parent = c
	v.label = c.label + " " + "chain" + " " + name
	return v
}

func (c *indexDBAccount) Resolve(key *record.Key) (record.Record, *record.Key, error) {
	if key.Len() == 0 {
		return nil, nil, errors.InternalError.With("bad key for account")
	}

	switch key.Get(0) {
	case "DidIndexTransactionExecution":
		return c.DidIndexTransactionExecution(), key.SliceI(1), nil
	case "DidLoadTransaction":
		return c.DidLoadTransaction(), key.SliceI(1), nil
	case "Chain":
		if key.Len() < 2 {
			return nil, nil, errors.InternalError.With("bad key for account")
		}
		name, okName := key.Get(1).(string)
		if !okName {
			return nil, nil, errors.InternalError.With("bad key for account")
		}
		v := c.Chain(name)
		return v, key.SliceI(2), nil
	default:
		return nil, nil, errors.InternalError.With("bad key for account")
	}
}

func (c *indexDBAccount) IsDirty() bool {
	if c == nil {
		return false
	}

	if values.IsDirty(c.didIndexTransactionExecution) {
		return true
	}
	if values.IsDirty(c.didLoadTransaction) {
		return true
	}
	for _, v := range c.chain {
		if v.IsDirty() {
			return true
		}
	}

	return false
}

func (c *indexDBAccount) Walk(opts record.WalkOptions, fn record.WalkFunc) error {
	if c == nil {
		return nil
	}

	skip, err := values.WalkComposite(c, opts, fn)
	if skip || err != nil {
		return errors.UnknownError.Wrap(err)
	}
	values.WalkField(&err, c.didIndexTransactionExecution, c.DidIndexTransactionExecution, opts, fn)
	values.WalkField(&err, c.didLoadTransaction, c.DidLoadTransaction, opts, fn)
	for _, v := range c.chain {
		values.Walk(&err, v, opts, fn)
	}
	return err
}

func (c *indexDBAccount) Commit() error {
	if c == nil {
		return nil
	}

	var err error
	values.Commit(&err, c.didIndexTransactionExecution)
	values.Commit(&err, c.didLoadTransaction)
	for _, v := range c.chain {
		values.Commit(&err, v)
	}

	return err
}

type IndexDBAccountChain interface {
	record.Record
	Index() values.List[*protocol.IndexEntry]
}

type indexDBAccountChain struct {
	logger logging.OptionalLogger
	store  record.Store
	key    *record.Key
	label  string
	parent *indexDBAccount

	index values.List[*protocol.IndexEntry]
}

func (c *indexDBAccountChain) Key() *record.Key { return c.key }

func (c *indexDBAccountChain) Index() values.List[*protocol.IndexEntry] {
	return values.GetOrCreate(&c.index, c.newIndex)
}

func (c *indexDBAccountChain) newIndex() values.List[*protocol.IndexEntry] {
	return values.NewList(c.logger.L, c.store, c.key.Append("Index"), c.label+" "+"index", values.Struct[protocol.IndexEntry]())
}

func (c *indexDBAccountChain) Resolve(key *record.Key) (record.Record, *record.Key, error) {
	if key.Len() == 0 {
		return nil, nil, errors.InternalError.With("bad key for chain")
	}

	switch key.Get(0) {
	case "Index":
		return c.Index(), key.SliceI(1), nil
	default:
		return nil, nil, errors.InternalError.With("bad key for chain")
	}
}

func (c *indexDBAccountChain) IsDirty() bool {
	if c == nil {
		return false
	}

	if values.IsDirty(c.index) {
		return true
	}

	return false
}

func (c *indexDBAccountChain) Walk(opts record.WalkOptions, fn record.WalkFunc) error {
	if c == nil {
		return nil
	}

	skip, err := values.WalkComposite(c, opts, fn)
	if skip || err != nil {
		return errors.UnknownError.Wrap(err)
	}
	if !opts.IgnoreIndices {
		values.WalkField(&err, c.index, c.Index, opts, fn)
	}
	return err
}

func (c *indexDBAccountChain) Commit() error {
	if c == nil {
		return nil
	}

	var err error
	values.Commit(&err, c.index)

	return err
}

type IndexDBPartition interface {
	record.Record
	Anchors() values.List[*AnchorMetadata]
}

type indexDBPartition struct {
	logger logging.OptionalLogger
	store  record.Store
	key    *record.Key
	label  string
	parent *indexDB

	anchors values.List[*AnchorMetadata]
}

func (c *indexDBPartition) Key() *record.Key { return c.key }

func (c *indexDBPartition) Anchors() values.List[*AnchorMetadata] {
	return values.GetOrCreate(&c.anchors, c.newAnchors)
}

func (c *indexDBPartition) newAnchors() values.List[*AnchorMetadata] {
	return values.NewList(c.logger.L, c.store, c.key.Append("Anchors"), c.label+" "+"anchors", values.Struct[AnchorMetadata]())
}

func (c *indexDBPartition) Resolve(key *record.Key) (record.Record, *record.Key, error) {
	if key.Len() == 0 {
		return nil, nil, errors.InternalError.With("bad key for partition")
	}

	switch key.Get(0) {
	case "Anchors":
		return c.Anchors(), key.SliceI(1), nil
	default:
		return nil, nil, errors.InternalError.With("bad key for partition")
	}
}

func (c *indexDBPartition) IsDirty() bool {
	if c == nil {
		return false
	}

	if values.IsDirty(c.anchors) {
		return true
	}

	return false
}

func (c *indexDBPartition) Walk(opts record.WalkOptions, fn record.WalkFunc) error {
	if c == nil {
		return nil
	}

	skip, err := values.WalkComposite(c, opts, fn)
	if skip || err != nil {
		return errors.UnknownError.Wrap(err)
	}
	values.WalkField(&err, c.anchors, c.Anchors, opts, fn)
	return err
}

func (c *indexDBPartition) Commit() error {
	if c == nil {
		return nil
	}

	var err error
	values.Commit(&err, c.anchors)

	return err
}

type IndexDBTransaction interface {
	record.Record
	Executed() values.Value[*EventMetadata]
}

type indexDBTransaction struct {
	logger logging.OptionalLogger
	store  record.Store
	key    *record.Key
	label  string
	parent *indexDB

	executed values.Value[*EventMetadata]
}

func (c *indexDBTransaction) Key() *record.Key { return c.key }

func (c *indexDBTransaction) Executed() values.Value[*EventMetadata] {
	return values.GetOrCreate(&c.executed, c.newExecuted)
}

func (c *indexDBTransaction) newExecuted() values.Value[*EventMetadata] {
	return values.NewValue(c.logger.L, c.store, c.key.Append("Executed"), c.label+" "+"executed", false, values.Struct[EventMetadata]())
}

func (c *indexDBTransaction) Resolve(key *record.Key) (record.Record, *record.Key, error) {
	if key.Len() == 0 {
		return nil, nil, errors.InternalError.With("bad key for transaction")
	}

	switch key.Get(0) {
	case "Executed":
		return c.Executed(), key.SliceI(1), nil
	default:
		return nil, nil, errors.InternalError.With("bad key for transaction")
	}
}

func (c *indexDBTransaction) IsDirty() bool {
	if c == nil {
		return false
	}

	if values.IsDirty(c.executed) {
		return true
	}

	return false
}

func (c *indexDBTransaction) Walk(opts record.WalkOptions, fn record.WalkFunc) error {
	if c == nil {
		return nil
	}

	skip, err := values.WalkComposite(c, opts, fn)
	if skip || err != nil {
		return errors.UnknownError.Wrap(err)
	}
	if !opts.IgnoreIndices {
		values.WalkField(&err, c.executed, c.Executed, opts, fn)
	}
	return err
}

func (c *indexDBTransaction) Commit() error {
	if c == nil {
		return nil
	}

	var err error
	values.Commit(&err, c.executed)

	return err
}
