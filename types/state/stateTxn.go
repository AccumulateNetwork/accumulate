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
	"github.com/AccumulateNetwork/accumulate/smt/pmt"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/smt/storage/database"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/tendermint/tendermint/libs/log"
	"sort"
	"sync"
	"time"
)

type DBTransactional struct {
	db           *database.Manager
	mm           *managed.MerkleManager
	debug        bool
	bpt          *pmt.Manager //pbt is the global patricia trie for the application
	blockIndex   int64        //Index of the current block
	TimeBucket   float64
	mutex        sync.Mutex
	updates      map[types.Bytes32]*blockUpdates
	writes       map[storage.Key][]byte
	transactions transactionLists
	sync         sync.WaitGroup
	logger       log.Logger
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

func (s *StateDB) Begin() *DBTransactional {
	dbTx := &DBTransactional{
		db:           s.db,
		logger:       s.logger,
		bpt:          s.bpt,
		mm:           s.mm,
		debug:        s.debug,
		mutex:        s.mutex,
		TimeBucket:   s.TimeBucket,
		sync:         s.sync,
		updates:      s.updates,
		transactions: s.transactions,
		writes:       s.writes,
		blockIndex:   s.blockIndex,
	}
	return dbTx
}

func (tx *DBTransactional) AddStateEntry(chainId *types.Bytes32, txHash *types.Bytes32, object *Object) {
	tx.logInfo("AddStateEntry", "chainId", logging.AsHex(chainId), "txHash", logging.AsHex(txHash), "entry", logging.AsHex(object.Entry))
	begin := time.Now()

	tx.TimeBucket = tx.TimeBucket + float64(time.Since(begin))*float64(time.Nanosecond)*1e-9

	tx.mutex.Lock()
	updates := tx.updates[*chainId]
	tx.mutex.Unlock()

	if updates == nil {
		updates = new(blockUpdates)
		tx.updates[*chainId] = updates
	}

	updates.txId = append(updates.txId, txHash)
	updates.stateData = object
}

//GetPersistentEntry will pull the data from the database for the StateEntries bucket.
func (tx *DBTransactional) GetPersistentEntry(chainId []byte, verify bool) (*Object, error) {
	_ = verify
	tx.Sync()

	if tx.db == nil {
		return nil, fmt.Errorf("database has not been initialized")
	}

	data, err := tx.db.Key("StateEntries", chainId).Get()
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

func (tx *DBTransactional) AddTransaction(chainId *types.Bytes32, txId types.Bytes, txPending, txAccepted *Object) error {
	var txAcceptedEntry []byte
	if txAccepted != nil {
		txAcceptedEntry = txAccepted.Entry
	}
	tx.logInfo("AddTransaction", "chainId", logging.AsHex(chainId), "txid", logging.AsHex(txId), "pending", logging.AsHex(txPending.Entry), "accepted", logging.AsHex(txAcceptedEntry))

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
	tx.mutex.Lock()
	defer tx.mutex.Unlock()

	tsi := transactionStateInfo{txPending, chainId.Bytes(), txId}
	tx.transactions.pendingTx = append(tx.transactions.pendingTx, &tsi)

	if txAccepted != nil {
		tsi := transactionStateInfo{txAccepted, chainId.Bytes(), txId}
		tx.transactions.validatedTx = append(tx.transactions.validatedTx, &tsi)
	}
	return nil
}

// GetCurrentEntry retrieves the current state object from the database based upon chainId.  Current state either comes
// from a previously saves state for the current block, or it is from the database
func (tx *DBTransactional) GetCurrentEntry(chainId []byte) (*Object, error) {
	if chainId == nil {
		return nil, fmt.Errorf("chain id is invalid, thus unable to retrieve current entry")
	}
	var ret *Object
	var err error
	var key types.Bytes32

	copy(key[:32], chainId[:32])

	tx.mutex.Lock()
	currentState := tx.updates[key]
	tx.mutex.Unlock()
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
func (tx *DBTransactional) AddSynthTx(parentTxId types.Bytes, synthTxId types.Bytes, synthTxObject *Object) {
	tx.logInfo("AddSynthTx", "txid", logging.AsHex(synthTxId), "entry", logging.AsHex(synthTxObject.Entry))
	var val *[]transactionStateInfo
	var ok bool

	parentHash := parentTxId.AsBytes32()
	if val, ok = tx.transactions.synthTxMap[parentHash]; !ok {
		val = new([]transactionStateInfo)
		tx.transactions.synthTxMap[parentHash] = val
	}
	*val = append(*val, transactionStateInfo{synthTxObject, nil, synthTxId})
}

func (tx *DBTransactional) writeTxs(mutex *sync.Mutex, group *sync.WaitGroup) error {
	defer group.Done()
	//record transactions
	for _, txn := range tx.transactions.validatedTx {
		data, _ := txn.Object.MarshalBinary()
		//store the transaction

		txHash := txn.TxId.AsBytes32()
		if synthTxInfos, ok := tx.transactions.synthTxMap[txHash]; ok {
			var synthData []byte
			for _, synthTxInfo := range *synthTxInfos {
				synthData = append(synthData, synthTxInfo.TxId...)
				synthTxData, err := synthTxInfo.Object.MarshalBinary()
				if err != nil {
					return err
				}

				tx.db.Key(bucketStagedSynthTx.AsString(), "", synthTxInfo.TxId).PutBatch(synthTxData)

				//store the hash of th synthObject in the bpt, will be removed after synth tx is processed
				tx.bpt.Bpt.Insert(synthTxInfo.TxId.AsBytes32(), sha256.Sum256(synthTxData))
			}
			//store a list of txid to list of synth txid's
			tx.db.Key(bucketTxToSynthTx.AsString(), txn.TxId).PutBatch(synthData)
		}

		mutex.Lock()
		//store the transaction in the transaction bucket by txid
		tx.db.Key(bucketTx.AsString(), txn.TxId).PutBatch(data)
		//insert the hash of the tx object in the BPT
		tx.bpt.Bpt.Insert(txHash, sha256.Sum256(data))
		mutex.Unlock()
	}

	// record pending transactions
	for _, txn := range tx.transactions.pendingTx {
		//marshal the pending transaction state
		data, _ := txn.Object.MarshalBinary()
		//hash it and add to the merkle state for the pending chain
		pendingHash := sha256.Sum256(data)

		mutex.Lock()
		//Store the mapping of the Transaction hash to the pending transaction hash which can be used for
		// validation so we can find the pending transaction
		tx.db.Key("MainToPending", txn.TxId).PutBatch(pendingHash[:])

		//store the pending transaction by the pending tx hash
		tx.db.Key(bucketPendingTx.AsString(), pendingHash[:]).PutBatch(data)
		mutex.Unlock()
	}

	//clear out the transactions after they have been processed
	tx.transactions.validatedTx = nil
	tx.transactions.pendingTx = nil
	tx.transactions.synthTxMap = make(map[types.Bytes32]*[]transactionStateInfo)
	return nil
}

func (tx *DBTransactional) Commit(blockHeight int64) ([]byte, int, error) {
	//Write states
	//build a list of keys from the map
	currentStateCount := len(tx.updates)
	if currentStateCount == 0 {
		//only attempt to record the block if we have any data.
		return tx.bpt.Bpt.Root.Hash[:], 0, nil
	}

	tx.blockIndex = blockHeight
	// TODO MainIndex and PendingIndex?
	tx.AddStateEntry((*types.Bytes32)(&blockIndexKey), new(types.Bytes32), &Object{Entry: common.Int64Bytes(blockHeight)})

	group := new(sync.WaitGroup)
	group.Add(1)
	group.Add(len(tx.updates))

	mutex := new(sync.Mutex)
	//to try the multi-threading add "go" in front of the next line
	err := tx.writeTxs(mutex, group)
	if err != nil {
		return tx.bpt.Bpt.Root.Hash[:], 0, nil
	}

	// Create an ordered list of chain IDs that need updating. The iteration
	// order of maps in Go is random. Randomly ordering database writes is bad,
	// because that leads to consensus errors between nodes, since each node
	// will have a different random order. So we need updates to have some
	// consistent order, regardless of what it is.
	updateOrder := make([]types.Bytes32, 0, len(tx.updates))
	for id := range tx.updates {
		updateOrder = append(updateOrder, id)
	}
	sort.Slice(updateOrder, func(i, j int) bool {
		return bytes.Compare(updateOrder[i][:], updateOrder[j][:]) < 0
	})

	for _, chainId := range updateOrder {
		tx.mm.SetChainID(chainId[:])
		tx.writeChainState(group, mutex, tx.mm, chainId)

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
	writeOrder := make([]storage.Key, 0, len(tx.writes))
	for k := range tx.writes {
		writeOrder = append(writeOrder, k)
	}
	sort.Slice(writeOrder, func(i, j int) bool {
		return bytes.Compare(writeOrder[i][:], writeOrder[j][:]) < 0
	})
	for _, k := range writeOrder {
		tx.GetDB().Key(k).PutBatch(tx.writes[k])
	}
	// The compiler optimizes this into a constant-time operation
	for k := range tx.writes {
		delete(tx.writes, k)
	}

	group.Wait()

	tx.bpt.Bpt.Update()

	//reset out block update buffer to get ready for the next round
	tx.sync.Add(1)
	//to enable threaded batch writes, put go in front of next line.
	tx.writeBatches()

	tx.updates = make(map[types.Bytes32]*blockUpdates)

	//return the state of the BPT for the state of the block
	rh := types.Bytes(tx.RootHash()).AsBytes32()
	tx.logInfo("WriteStates", "height", blockHeight, "hash", logging.AsHex(rh))
	return tx.RootHash(), currentStateCount, nil
	panic("TODO")
}

func (tx *DBTransactional) writeChainState(group *sync.WaitGroup, mutex *sync.Mutex, mm *managed.MerkleManager, chainId types.Bytes32) {
	defer group.Done()

	// We get ChainState objects here, instead. And THAT will hold
	//       the MerkleStateManager for the chain.
	//mutex.Lock()
	currentState := tx.updates[chainId]
	//mutex.Unlock()

	if currentState == nil {
		panic(fmt.Sprintf("Chain state is nil meaning no updates were stored on chain %X for the block. Should not get here!", chainId[:]))
	}

	//add all the transaction states that occurred during this block for this chain (in order of appearance)
	for _, txn := range currentState.txId {
		//store the txHash for the chains, they will be mapped back to the above recorded tx's
		tx.logInfo("AddHash", "hash", logging.AsHex(txn))
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
		tx.GetDB().Key(bucketEntry.AsString(), chainId.Bytes()).PutBatch(chainStateObject)
		// The bpt stores the hash of the ChainState object hash.
		tx.bpt.Bpt.Insert(chainId, sha256.Sum256(chainStateObject))
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

func (tx *DBTransactional) logInfo(msg string, keyVals ...interface{}) {
	if tx.logger != nil {
		// TODO Maybe this should be Debug?
		tx.logger.Info(msg, keyVals...)
	}
}

func (tx *DBTransactional) GetDB() *database.Manager {
	return tx.db
}

func (tx *DBTransactional) Sync() {
	tx.sync.Wait()
}

func (tx *DBTransactional) RootHash() []byte {
	h := tx.bpt.Bpt.Root.Hash // Make a copy
	return h[:]               // Return a reference to the copy
}

func (tx *DBTransactional) writeBatches() {
	defer tx.sync.Done()
	tx.db.EndBatch()
	tx.bpt.DBManager.EndBatch()
}
