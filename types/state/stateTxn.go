package state

import (
	"errors"
	"fmt"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/smt/storage/database"
	"github.com/AccumulateNetwork/accumulate/types"
)

type DBTransaction struct {
	state        *StateDB
	updates      map[types.Bytes32]*blockUpdates
	writes       map[storage.Key][]byte
	transactions transactionLists
}

func (tx *DBTransaction) WriteStates(blockHeight int64) ([]byte, int, error) {
	panic("implement me")
}

func (s *StateDB) Begin() *DBTransaction {
	dbTx := &DBTransaction{
		state: s,
	}
	dbTx.updates = make(map[types.Bytes32]*blockUpdates)
	dbTx.writes = map[storage.Key][]byte{}
	dbTx.transactions.reset()
	return dbTx
}

//GetPersistentEntry will pull the data from the database for the StateEntries bucket.
func (tx *DBTransaction) GetPersistentEntry(chainId []byte, verify bool) (*Object, error) {
	_ = verify
	tx.state.Sync()

	if tx.state.db == nil {
		return nil, fmt.Errorf("database has not been initialized")
	}

	data, err := tx.state.db.Key("StateEntries", chainId).Get()
	if errors.Is(err, storage.ErrNotFound) {
		return nil, fmt.Errorf("%w: no state defined for %X", storage.ErrNotFound, chainId)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get state entry %X: %v", chainId, err)
	}

	ret := &Object{}
	err = ret.UnmarshalBinary(data)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal state for %x", chainId)
	}
	//if verify {
	//todo: generate and verify data to make sure the state matches what is in the patricia trie
	//}
	return ret, nil
}

func (tx *DBTransaction) GetDB() *database.Manager {
	return tx.state.GetDB()
}

func (tx *DBTransaction) Sync() {
	tx.state.Sync()
}

func (tx *DBTransaction) RootHash() []byte {
	return tx.state.RootHash()
}

func (tx *DBTransaction) BlockIndex() int64 {
	return tx.state.BlockIndex()
}

func (tx *DBTransaction) EnsureRootHash() []byte {
	return tx.state.EnsureRootHash()
}
