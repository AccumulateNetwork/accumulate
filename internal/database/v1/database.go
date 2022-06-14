package database

import (
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type Database = database.Database
type Batch = database.ChangeSet
type Account = database.Account
type Chain = database.ChainV1
type Transaction = database.Transaction
type SignatureSet = database.SignatureSet
type SigOrTxn = database.SigOrTxn
type SigSetEntry = database.SignatureEntry

func New(store storage.KeyValueStore, logger log.Logger) *Database {
	return database.New(store, logger)
}

func OpenInMemory(logger log.Logger) *Database {
	return database.OpenInMemory(logger)
}

func OpenBadger(filepath string, logger log.Logger) (*Database, error) {
	return database.OpenBadger(filepath, logger)
}

func OpenEtcd(prefix string, config *clientv3.Config, logger log.Logger) (*Database, error) {
	return database.OpenEtcd(prefix, config, logger)
}

func Open(cfg *config.Config, logger log.Logger) (*Database, error) {
	return database.Open(cfg, logger)
}

func ReadSnapshot(file ioutil2.SectionReader) (height uint64, format uint32, bptRoot []byte, rd ioutil2.SectionReader, err error) {
	return database.ReadSnapshot(file)
}
