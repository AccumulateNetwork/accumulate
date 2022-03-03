package etcd

import (
	"context"
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/memory"
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
	return &storage.DebugBatch{Batch: b, Logger: db.logger}
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
		return nil, storage.ErrNotFound
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
		return errors.New("transaction failed")
	}
	return nil
}
