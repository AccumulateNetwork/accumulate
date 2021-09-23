package state

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"github.com/AccumulateNetwork/accumulated/types"

	"github.com/AccumulateNetwork/SMT/managed"
	"github.com/AccumulateNetwork/SMT/pmt"
	smtDB "github.com/AccumulateNetwork/SMT/storage/database"
)

type transactionLists struct {
	validatedTx []*Transaction
	pendingTx   []*PendingTransaction
}

type blockUpdates struct {
	txId      []*types.Bytes32
	stateData *Object //the latest chain state object modified from a tx
}

// StateDB the state DB will only retrieve information out of the database.  To store stuff use PersistentStateDB instead
type StateDB struct {
	db    *smtDB.Manager
	debug bool
	// TODO:  Need a couple of things:
	//     ChainState interface
	//     Lets you marshal a ChainState to disk, and lets you unmarshal them
	//     later.  Holds the type of the chain, and all the state about the
	//     chain needed to validate transactions on the chain. (accounts for
	//     example Balance, SigSpecGroup (hash), URL)  All ChainState
	//     instances hold the MerkleManagerState for the chain. MDRoot
	//     .
	//     On the other side, allows a ChainState to be unmarshaled for updates
	//     and access.
	bpt   *pmt.Manager           //pbt is the global patricia trie for the application
	mm    *managed.MerkleManager //mm is the merkle manager for the application.  The salt is set by the appId
	appId []byte

	TimeBucket   float64
	mutex        sync.Mutex
	updates      map[types.Bytes32]*blockUpdates
	transactions transactionLists
	sync         sync.WaitGroup
}

// Open database to manage the smt and chain states
func (sdb *StateDB) Open(dbFilename string, appId []byte, useMemDB bool, debug bool) error {
	dbType := "badger"
	markPower := int64(8)
	if useMemDB {
		dbType = "memory"
	}

	sdb.db = &smtDB.Manager{}
	err := sdb.db.Init(dbType, dbFilename)
	if err != nil {
		return err
	}

	sdb.db.AddBucket("StateEntries")
	sdb.db.AddBucket("MainToPending")
	sdb.db.AddBucket("PendingTx")
	sdb.db.AddBucket("Tx")
	sdb.debug = debug
	if debug {
		sdb.db.AddBucket("Entries-Debug") //items will bet pushed into this bucket as the state entries change
	}

	sdb.updates = make(map[types.Bytes32]*blockUpdates)

	sdb.bpt = pmt.NewBPTManager(sdb.db)
	sdb.mm = managed.NewMerkleManager(sdb.db, appId, markPower)
	sdb.appId = appId
	return nil
}

func (sdb *StateDB) GetDB() *smtDB.Manager {
	return sdb.db
}

func (sdb *StateDB) Sync() {
	sdb.sync.Wait()
}

//AddPendingTx adds the pending tx raw data and signature of that data to tx, signature needs to be a signed hash of the tx.
func (sdb *StateDB) AddPendingTx(chainId *types.Bytes32, txPending *PendingTransaction, txValidated *Transaction) error {
	_ = chainId
	//append the list of pending Tx's, txId's, and validated Tx's.
	sdb.mutex.Lock()
	sdb.transactions.pendingTx = append(sdb.transactions.pendingTx, txPending)
	if txValidated != nil {
		sdb.transactions.validatedTx = append(sdb.transactions.validatedTx, txValidated)
	}
	sdb.mutex.Unlock()
	return nil
}

//GetPersistentEntry will pull the data from the database for the StateEntries bucket.
func (sdb *StateDB) GetPersistentEntry(chainId []byte, verify bool) (*Object, error) {
	sdb.Sync()

	if sdb.db == nil {
		return nil, fmt.Errorf("database has not been initialized")
	}

	data := sdb.db.Get("StateEntries", "", chainId)

	if data == nil {
		return nil, fmt.Errorf("no current state is defined")
	}

	ret := &Object{}
	err := ret.UnmarshalBinary(data)
	if err != nil {
		return nil, fmt.Errorf("entry in database is not found for %x", chainId)
	}
	//if verify {
	//todo: generate and verify data to make sure the state matches what is in the patricia trie
	//}
	return ret, nil
}

// GetCurrentEntry retrieves the current state object from the database based upon chainId.  Current state either comes
// from a previously saves state for the current block, or it is from the database
func (sdb *StateDB) GetCurrentEntry(chainId []byte) (*Object, error) {

	if chainId == nil {
		return nil, fmt.Errorf("chain id is invalid, thus unable to retrieve current entry")
	}
	var ret *Object
	var err error
	var key types.Bytes32

	copy(key[:32], chainId[:32])

	sdb.mutex.Lock()
	currentState := sdb.updates[key]
	sdb.mutex.Unlock()
	if currentState != nil {
		ret = currentState.stateData
	} else {
		currentState := blockUpdates{}
		//pull current state entry from the database.
		currentState.stateData, err = sdb.GetPersistentEntry(chainId, false)
		if err != nil {
			return nil, fmt.Errorf("no current state is defined, %v", err)
		}
		//if we have valid data, store off the state
		ret = currentState.stateData
	}

	return ret, nil
}

// AddStateEntry append the entry to the chain, the subChainId is if the chain upon which
// the transaction is against touches another chain. One example would be an account type chain
// may change the state of the sigspecgroup chain (i.e. a sub/secondary chain) based on the effect
// of a transaction.  The entry is the state object associated with
func (sdb *StateDB) AddStateEntry(chainId *types.Bytes32, txHash *types.Bytes32, entry []byte) error {
	begin := time.Now()

	sdb.TimeBucket = sdb.TimeBucket + float64(time.Since(begin))*float64(time.Nanosecond)*1e-9

	sdb.mutex.Lock()
	updates := sdb.updates[*chainId]
	sdb.mutex.Unlock()

	if updates == nil {
		updates = new(blockUpdates)
		sdb.updates[*chainId] = updates
	}
	if updates.stateData == nil {
		updates.stateData = new(Object)
	}

	updates.txId = append(updates.txId, txHash)
	updates.stateData.Entry = entry

	return nil
}

func (sdb *StateDB) writeTxs(group *sync.WaitGroup) {
	defer group.Done()
	//record transactions
	for _, tx := range sdb.transactions.validatedTx {
		data, _ := tx.MarshalBinary()
		//store the transaction
		err := sdb.mm.RootDBManager.PutBatch("Tx", "", tx.TransactionHash().Bytes(), data)
		if err != nil {
			panic("failed to put batch for validated transaction write")
		}
	}

	//record pending transactions
	for _, tx := range sdb.transactions.pendingTx {
		//marshal the pending transaction state
		data, _ := tx.MarshalBinary()
		//hash it and add to the merkle state for the pending chain
		pendingHash := sha256.Sum256(data)

		//Store the mapping of the Transaction hash to the pending transaction hash which can be used for validation so we can find
		//the pending transaction
		err := sdb.mm.RootDBManager.PutBatch("MainToPending", "", tx.TransactionState.transactionHash.Bytes(), pendingHash[:])
		if err != nil {
			panic("failed to put batch \"MainToPending\" for transaction write")
		}

		//store the pending transaction by the pending tx hash
		err = sdb.mm.RootDBManager.PutBatch("PendingTx", "", pendingHash[:], data)
		if err != nil {
			panic("failed to put batch \"PendingTx\" for transaction write")
		}
	}

	//clear out the transactions after they have been processed
	sdb.transactions.validatedTx = nil
	sdb.transactions.pendingTx = nil
}

func (sdb *StateDB) writeChainState(group *sync.WaitGroup, mutex *sync.Mutex, mm *managed.MerkleManager, chainId types.Bytes32) {
	defer group.Done()

	v := mm
	// We get ChainState objects here, instead. And THAT will hold
	//       the MerkleStateManager for the chain.
	//mutex.Lock()
	currentState := sdb.updates[chainId]
	//mutex.Unlock()

	if currentState == nil {
		panic(fmt.Sprintf("Chain state is nil meaning no updates were stored on chain %X for the block. Should not get here!", chainId[:]))
	}

	//add all the transaction states that occurred during this block for this chain (in order of appearance)
	for _, tx := range currentState.txId {
		//store the txHash for the chains, they will be mapped back to the above recorded tx's
		v.AddHash(managed.Hash(*tx))
	}

	if currentState.stateData != nil {
		//store the MD root for the state
		mdRoot := v.MainChain.MS.GetMDRoot()
		if mdRoot == nil {
			//shouldn't get here, but will reject if I do
			panic(fmt.Sprintf("shouldn't get here on writeState() on chain id %X obtaining merkle state", chainId))
		}

		//store the state of the main chain in the state object
		currentState.stateData.MDRoot = mdRoot[:]

		//now store the state object
		chainStateObject, err := currentState.stateData.MarshalBinary()
		if err != nil {
			panic("failed to marshal binary for state data")
		}

		err = sdb.GetDB().PutBatch("StateEntries", "", chainId.Bytes(), chainStateObject)

		if err != nil {
			//shouldn't get here, and bad if I do...
			panic(fmt.Sprintf("failed to store data entry in StateEntries bucket, %v", err))
		}
		mutex.Lock()
		// The bpt stores the hash of the ChainState object hash.
		sdb.bpt.Bpt.Insert(chainId, sha256.Sum256(chainStateObject))
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
	//	sdb.bpt.Bpt.Insert(chainId, *mdRoot)
	//}
}

func (sdb *StateDB) writeBatches() {
	defer sdb.sync.Done()
	sdb.mm.RootDBManager.EndBatch()
}

// WriteStates will push the data to the database and update the patricia trie
func (sdb *StateDB) WriteStates(blockHeight int64) ([]byte, int, error) {
	//build a list of keys from the map
	currentStateCount := len(sdb.updates)
	if currentStateCount == 0 {
		//only attempt to record the block if we have any data.
		return sdb.bpt.Bpt.Root.Hash[:], 0, nil
	}

	sdb.mm.SetBlockIndex(blockHeight)

	group := new(sync.WaitGroup)
	group.Add(1)
	group.Add(len(sdb.updates))
	sdb.writeTxs(group)
	//go sdb.writeTxs(group)
	//
	//keys := make([]types.Bytes32, 0, currentStateCount)
	//for chainId := range sdb.updates {
	//	keys = append(keys, chainId)
	//}
	//
	////sort the keys
	//sort.Slice(keys, func(i, j int) bool {
	//	return bytes.Compare(keys[i][:], keys[j][:]) < 0
	//})

	mutex := new(sync.Mutex)
	//then run through the list and record them
	//loop through everything and write out states to the database.

	merkleMgrMap := make(map[types.Bytes32]*managed.MerkleManager)
	for chainId := range sdb.updates {
		merkleMgrMap[chainId] = sdb.mm.Copy(chainId[:])
	}
	//for _, chainId := range keys {
	for chainId := range sdb.updates {

		sdb.writeChainState(group, mutex, merkleMgrMap[chainId], chainId)

		//TODO: figure out how to do this with new way state is derived
		//if len(currentState.pendingTx) != 0 {
		//	mdRoot := v.PendingChain.MS.GetMDRoot()
		//	if mdRoot == nil {
		//		//shouldn't get here, but will reject if I do
		//		panic(fmt.Sprintf("shouldn't get here on writeState() on chain id %X obtaining merkle state", chainId))
		//	}
		//	//todo:  Determine how we purge pending tx's after 2 weeks.
		//	sdb.bpt.Bpt.Insert(chainId, *mdRoot)
		//}
	}
	group.Wait()

	sdb.bpt.Bpt.Update()

	//reset out block update buffer to get ready for the next round
	sdb.sync.Add(1)
	//go sdb.writeBatches()
	sdb.writeBatches()
	sdb.updates = make(map[types.Bytes32]*blockUpdates)

	//return the state of the BPT for the state of the block
	return sdb.bpt.Bpt.Root.Hash[:], currentStateCount, nil
}
