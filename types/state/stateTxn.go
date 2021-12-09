package state

import (
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/smt/storage/database"
	"github.com/AccumulateNetwork/accumulate/types"
)

type DBTransaction struct {
	state        *StateDB
	dataUpdates  map[types.Bytes32]*dataBlockUpdates
	updates      map[types.Bytes32]*blockUpdates
	writes       map[storage.Key][]byte
	addSynthSigs []*SyntheticSignature
	delSynthSigs [][32]byte
	transactions transactionLists
}

func (s *StateDB) Begin() *DBTransaction {
	dbTx := &DBTransaction{
		state: s,
	}
	dbTx.updates = make(map[types.Bytes32]*blockUpdates)
	dbTx.dataUpdates = make(map[types.Bytes32]*dataBlockUpdates)
	dbTx.writes = map[storage.Key][]byte{}
	dbTx.transactions.reset()
	return dbTx
}

// DB returns the transaction's database.
func (tx *DBTransaction) DB() *StateDB { return tx.state }

func (tx *DBTransaction) AddSynthTxnSig(sig *SyntheticSignature) {
	tx.addSynthSigs = append(tx.addSynthSigs, sig)
}

func (tx *DBTransaction) DeleteSynthTxnSig(txid [32]byte) {
	tx.delSynthSigs = append(tx.delSynthSigs, txid)
}

// GetPersistentEntry calls StateDB.GetPersistentEntry(...).
func (tx *DBTransaction) GetPersistentEntry(chainId []byte, verify bool) (*Object, error) {
	return tx.state.GetPersistentEntry(chainId, verify)
}

// GetDB calls StateDB.GetDB().
func (tx *DBTransaction) GetDB() *database.Manager {
	return tx.state.GetDB()
}

// Sync calls StateDB.Sync().
func (tx *DBTransaction) Sync() {
	tx.state.Sync()
}

// RootHash calls StateDB.RootHash().
func (tx *DBTransaction) RootHash() []byte {
	return tx.state.RootHash()
}

// BlockIndex calls StateDB.BlockIndex().
func (tx *DBTransaction) BlockIndex() (int64, error) {
	return tx.state.BlockIndex()
}

// EnsureRootHash calls StateDB.EnsureRootHash().
func (tx *DBTransaction) EnsureRootHash() []byte {
	return tx.state.EnsureRootHash()
}
