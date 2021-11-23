package state

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/AccumulateNetwork/accumulate/internal/logging"
	"github.com/AccumulateNetwork/accumulate/smt/common"
	"github.com/AccumulateNetwork/accumulate/smt/managed"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/smt/storage/database"
	"github.com/AccumulateNetwork/accumulate/types"
	"sort"
	"sync"
)

type DBTransaction struct {
	state *StateDB
}

//func (tx *DBTransactional) init(debug bool) (err error) {
//
//	tx.debug = debug
//	tx.updates = make(map[types.Bytes32]*blockUpdates)
//	tx.writes = map[storage.Key][]byte{}
//	tx.transactions.reset()
//
//	tx.bpt = pmt.NewBPTManager(tx.db)
//	managed.NewMerkleManager(tx.db, markPower)
//
//	ent, err := tx.GetPersistentEntry(blockIndexKey[:], false)
//	if err == nil {
//		tx.blockIndex, _ = common.BytesInt64(ent.Entry)
//	} else if !errors.Is(err, storage.ErrNotFound) {
//		return err
//	}
//
//	return nil
//}

func (s *StateDB) Begin() *DBTransaction {
	dbTx := &DBTransaction{
		state: s,
	}
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

func (tx *DBTransaction) AddTransaction(chainId *types.Bytes32, txId types.Bytes, txPending, txAccepted *Object) error {
	var txAcceptedEntry []byte
	if txAccepted != nil {
		txAcceptedEntry = txAccepted.Entry
	}
	tx.state.logInfo("AddTransaction", "chainId", logging.AsHex(chainId), "txid", logging.AsHex(txId), "pending", logging.AsHex(txPending.Entry), "accepted", logging.AsHex(txAcceptedEntry))

	chainType, _ := binary.Uvarint(txPending.Entry)
	if types.ChainType(chainType) != types.ChainTypePendingTransaction {
		return fmt.Errorf("expecting pending transaction chain type of %s, but received %s",
			types.ChainTypePendingTransaction.Name(), types.TxType(chainType).Name())
	}

	if txAccepted != nil {
		chainType, _ = binary.Uvarint(txAccepted.Entry)
		if types.ChainType(chainType) != types.ChainTypeTransaction {
			return fmt.Errorf("expecting pending transaction chain type of %s, but received %s",
				types.ChainTypeTransaction.Name(), types.ChainType(chainType).Name())
		}
	}

	//append the list of pending Tx's, txId's, and validated Tx's.
	tx.state.mutex.Lock()
	defer tx.state.mutex.Unlock()

	tsi := transactionStateInfo{txPending, chainId.Bytes(), txId}
	tx.state.transactions.pendingTx = append(tx.state.transactions.pendingTx, &tsi)

	if txAccepted != nil {
		tsi := transactionStateInfo{txAccepted, chainId.Bytes(), txId}
		tx.state.transactions.validatedTx = append(tx.state.transactions.validatedTx, &tsi)
	}
	return nil
}

// GetCurrentEntry retrieves the current state object from the database based upon chainId.  Current state either comes
// from a previously saves state for the current block, or it is from the database
func (tx *DBTransaction) GetCurrentEntry(chainId []byte) (*Object, error) {
	if chainId == nil {
		return nil, fmt.Errorf("chain id is invalid, thus unable to retrieve current entry")
	}
	var ret *Object
	var err error
	var key types.Bytes32

	copy(key[:32], chainId[:32])

	tx.state.mutex.Lock()
	currentState := tx.state.updates[key]
	tx.state.mutex.Unlock()
	if currentState != nil {
		ret = currentState.stateData
	} else {
		currentState := blockUpdates{}
		currentState.bucket = bucketEntry
		//pull current state entry from the database.
		currentState.stateData, err = tx.GetPersistentEntry(chainId, false)
		if err != nil {
			return nil, err
		}
		//if we have valid data, store off the state
		ret = currentState.stateData
	}

	return ret, nil
}

//AddSynthTx add the synthetic transaction which is mapped to the parent transaction
func (tx *DBTransaction) AddSynthTx(parentTxId types.Bytes, synthTxId types.Bytes, synthTxObject *Object) {
	tx.state.logInfo("AddSynthTx", "txid", logging.AsHex(synthTxId), "entry", logging.AsHex(synthTxObject.Entry))
	var val *[]transactionStateInfo
	var ok bool

	parentHash := parentTxId.AsBytes32()
	if val, ok = tx.state.transactions.synthTxMap[parentHash]; !ok {
		val = new([]transactionStateInfo)
		tx.state.transactions.synthTxMap[parentHash] = val
	}
	*val = append(*val, transactionStateInfo{synthTxObject, nil, synthTxId})
}

func (tx *DBTransaction) Commit(blockHeight int64) ([]byte, int, error) {
	//Write states
	//build a list of keys from the map
	currentStateCount := len(tx.state.updates)
	if currentStateCount == 0 {
		//only attempt to record the block if we have any data.
		return tx.state.bpt.Bpt.Root.Hash[:], 0, nil
	}

	tx.state.blockIndex = blockHeight
	// TODO MainIndex and PendingIndex?
	tx.AddStateEntry((*types.Bytes32)(&blockIndexKey), new(types.Bytes32), &Object{Entry: common.Int64Bytes(blockHeight)})

	group := new(sync.WaitGroup)
	group.Add(1)
	group.Add(len(tx.state.updates))

	mutex := new(sync.Mutex)
	//to try the multi-threading add "go" in front of the next line
	err := tx.writeTxs(mutex, group)
	if err != nil {
		return tx.state.bpt.Bpt.Root.Hash[:], 0, nil
	}

	// Create an ordered list of chain IDs that need updating. The iteration
	// order of maps in Go is random. Randomly ordering database writes is bad,
	// because that leads to consensus errors between nodes, since each node
	// will have a different random order. So we need updates to have some
	// consistent order, regardless of what it is.
	updateOrder := make([]types.Bytes32, 0, len(tx.state.updates))
	for id := range tx.state.updates {
		updateOrder = append(updateOrder, id)
	}
	sort.Slice(updateOrder, func(i, j int) bool {
		return bytes.Compare(updateOrder[i][:], updateOrder[j][:]) < 0
	})

	for _, chainId := range updateOrder {
		tx.state.mm.SetChainID(chainId[:])
		tx.state.writeChainState(group, mutex, tx.state.mm, chainId)

		//TODO: figure out how to do this with new way state is derived
		//if len(currentState.pendingTx) != 0 {
		//	mdRoot := v.PendingChain.MS.GetMDRoot()
		//	if mdRoot == nil {
		//		//shouldn't get here, but will reject if I do
		//		panic(fmt.Sprintf("shouldn't get here on writeState() on chain id %X obtaining merkle state", chainId))
		//	}
		//	//todo:  Determine how we purge pending tx's after 2 weeks.
		//	s.bpt.Bpt.Insert(chainId, *mdRoot)
		//}
	}

	// Process pending writes
	writeOrder := make([]storage.Key, 0, len(tx.state.writes))
	for k := range tx.state.writes {
		writeOrder = append(writeOrder, k)
	}
	sort.Slice(writeOrder, func(i, j int) bool {
		return bytes.Compare(writeOrder[i][:], writeOrder[j][:]) < 0
	})
	for _, k := range writeOrder {
		tx.state.GetDB().Key(k).PutBatch(tx.state.writes[k])
	}
	// The compiler optimizes this into a constant-time operation
	for k := range tx.state.writes {
		delete(tx.state.writes, k)
	}

	group.Wait()

	tx.state.bpt.Bpt.Update()

	//reset out block update buffer to get ready for the next round
	tx.state.sync.Add(1)
	//to enable threaded batch writes, put go in front of next line.
	tx.state.writeBatches()

	tx.state.updates = make(map[types.Bytes32]*blockUpdates)

	//return the state of the BPT for the state of the block
	rh := types.Bytes(tx.state.RootHash()).AsBytes32()
	tx.state.logInfo("WriteStates", "height", blockHeight, "hash", logging.AsHex(rh))
	return tx.state.RootHash(), currentStateCount, nil
	panic("TODO")
}

func (tx *DBTransaction) writeChainState(group *sync.WaitGroup, mutex *sync.Mutex, mm *managed.MerkleManager, chainId types.Bytes32) {
	defer group.Done()

	// We get ChainState objects here, instead. And THAT will hold
	//       the MerkleStateManager for the chain.
	//mutex.Lock()
	currentState := tx.state.updates[chainId]
	//mutex.Unlock()

	if currentState == nil {
		panic(fmt.Sprintf("Chain state is nil meaning no updates were stored on chain %X for the block. Should not get here!", chainId[:]))
	}

	//add all the transaction states that occurred during this block for this chain (in order of appearance)
	for _, txn := range currentState.txId {
		//store the txHash for the chains, they will be mapped back to the above recorded tx's
		tx.state.logInfo("AddHash", "hash", logging.AsHex(txn))
		mm.AddHash(managed.Hash(*txn))
	}

	if currentState.stateData != nil {
		//store the MD root for the state
		mdRoot := mm.MS.GetMDRoot()
		if mdRoot == nil {
			//shouldn't get here, but will reject if I do
			panic(fmt.Sprintf("shouldn't get here on writeState() on chain id %X obtaining merkle state", chainId))
		}

		//store the state of the main chain in the state object
		currentState.stateData.MDRoot = types.Bytes32(*mdRoot)

		//now store the state object
		chainStateObject, err := currentState.stateData.MarshalBinary()
		if err != nil {
			panic("failed to marshal binary for state data")
		}

		mutex.Lock()
		tx.state.GetDB().Key(bucketEntry.AsString(), chainId.Bytes()).PutBatch(chainStateObject)
		// The bpt stores the hash of the ChainState object hash.
		tx.state.bpt.Bpt.Insert(chainId, sha256.Sum256(chainStateObject))
		mutex.Unlock()
	}
	//TODO: figure out how to do this with new way state is derived
	//if len(currentState.pendingTx) != 0 {
	//	mdRoot := v.PendingChain.MS.GetMDRoot()
	//	if mdRoot == nil {
	//		//shouldn't get here, but will reject if I do
	//		panic(fmt.Sprintf("shouldn't get here on writeState() on chain id %X obtaining merkle state", chainId))
	//	}
	//	//todo:  Determine how we purge pending tx's after 2 weeks.
	//	s.bpt.Bpt.Insert(chainId, *mdRoot)
	//}
}

func (tx *DBTransaction) logInfo(msg string, keyVals ...interface{}) {
	if tx.state.logger != nil {
		// TODO Maybe this should be Debug?
		tx.state.logger.Info(msg, keyVals...)
	}
}

func (tx *DBTransaction) GetDB() *database.Manager {
	return tx.state.db
}

func (tx *DBTransaction) Sync() {
	tx.state.sync.Wait()
}

func (tx *DBTransaction) RootHash() []byte {
	h := tx.state.bpt.Bpt.Root.Hash // Make a copy
	return h[:]                     // Return a reference to the copy
}

func (tx *DBTransaction) writeBatches() {
	defer tx.state.sync.Done()
	tx.state.db.EndBatch()
	tx.state.bpt.DBManager.EndBatch()
}

func (tx *DBTransaction) BlockIndex() int64 {
	return tx.state.blockIndex
}

func (tx *DBTransaction) EnsureRootHash() []byte {
	tx.state.bpt.Bpt.EnsureRootHash()
	return tx.state.RootHash()
}
