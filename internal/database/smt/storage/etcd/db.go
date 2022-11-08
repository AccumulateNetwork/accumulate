// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package etcd

import (
	"context"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage/memory"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	etcd "go.etcd.io/etcd/client/v3"
)

type DB struct {
	prefix string
	client *etcd.Client
	logger storage.Logger
}

func New(prefix string, config *etcd.Config, logger storage.Logger) (*DB, error) {
	// TODO Pass on the logger
	client, err := etcd.New(*config)
	if err != nil {
		return nil, err
	}

	db := new(DB)
	db.prefix = prefix
	db.client = client
	db.logger = logger
	return db, nil
}

var _ storage.KeyValueStore = (*DB)(nil)

func (db *DB) Close() error {
	return db.client.Close()
}

func (db *DB) Begin(writable bool) storage.KeyValueTxn {
	b := memory.NewBatch(db.get, db.commit)
	if db.logger == nil {
		return b
	}
	return &storage.DebugBatch{Batch: b, Logger: db.logger, Writable: writable}
}

func (db *DB) key(key storage.Key) string {
	return fmt.Sprintf("%s.%x", db.prefix, key)
}

func (db *DB) get(key storage.Key) ([]byte, error) {
	resp, err := db.client.Get(context.Background(), db.key(key))
	if err != nil {
		return nil, err
	}

	if resp.Count == 0 {
		return nil, errors.NotFound("key %v not found", key)
	}

	if resp.Count != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", resp.Count)
	}

	return resp.Kvs[0].Value, nil
}

func (db *DB) commit(batch map[storage.Key][]byte) error {
	ops := make([]etcd.Op, 0, len(batch))
	for k, v := range batch {
		ops = append(ops, etcd.OpPut(db.key(k), string(v)))
	}

	resp, err := db.client.Txn(context.Background()).Then(ops...).Commit()
	if err != nil {
		return err
	}
	if !resp.Succeeded {
		return errors.New(errors.StatusInternalError, "transaction failed")
	}
	return nil
}
