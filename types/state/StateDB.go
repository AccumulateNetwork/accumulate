package state

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/AccumulateNetwork/accumulated/types"

	"github.com/AccumulateNetwork/SMT/managed"
	"github.com/AccumulateNetwork/SMT/pmt"
	smtDB "github.com/AccumulateNetwork/SMT/storage/database"
)

type merkleManagerState struct {
	merkleMgr  *managed.MerkleManager //merkleMgr manages the merkle state for the chain
	stateEntry Object                 //stateEntry is the latest state entry for the current height
	pending    []*pendingValidationState
}

type pendingTxState struct {
	tx   []byte
	txId *types.Bytes32
}

type validatedTxState struct {
	tx   []byte
	txId *types.Bytes32
}

type pendingValidationState struct {
	pendingTx *pendingTxState
	txId      []byte // hash of the transaction, can be validated
	pubKey    []byte // public key used to sign txId
	sig       []byte // signature of pending validation, this is keyed by sha256(pendingTxState.tx)
}

type blockUpdates struct {
	validatedTx []*Transaction
	pendingTx   []*PendingTransaction
	stateData   *Object
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
	mms   map[managed.Hash]*merkleManagerState //mms is the merkle manager state map cached for the block, it is reset after each call to WriteState
	bpt   *pmt.Manager                         //pbt is the global patricia trie for the application
	mm    *managed.MerkleManager               //mm is the merkle manager for the application.  The salt is set by the appId
	appId []byte

	TimeBucket float64
	mutex      sync.Mutex
	updates    map[types.Bytes32]*blockUpdates
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

	sdb.mms = make(map[managed.Hash]*merkleManagerState)

	sdb.updates = make(map[types.Bytes32]*blockUpdates)

	sdb.bpt = pmt.NewBPTManager(sdb.db)
	sdb.mm = managed.NewMerkleManager(sdb.db, appId, markPower)
	sdb.appId = appId
	return nil
}

func (sdb *StateDB) GetDB() *smtDB.Manager {
	return sdb.db
}

//AddPendingTx adds the pending tx raw data and signature of that data to tx, signature needs to be a signed hash of the tx.
func (sdb *StateDB) AddPendingTx(chainId *types.Bytes32, txPending *PendingTransaction, txValidated *Transaction) error {
	var bu *blockUpdates

	//the locking assuming we are threaded per identity which means all chains will be processed in same thread
	sdb.mutex.Lock()
	if bu = sdb.updates[*chainId]; bu == nil {
		bu = new(blockUpdates)
		sdb.updates[*chainId] = bu
	}
	sdb.mutex.Unlock()

	//append the list of pending Tx's, txId's, and validated Tx's.
	bu.pendingTx = append(bu.pendingTx, txPending)
	if txValidated != nil {
		bu.validatedTx = append(bu.validatedTx, txValidated)
	}

	return nil
}

//GetPersistentEntry will pull the data from the database for the StateEntries bucket.
func (sdb *StateDB) GetPersistentEntry(chainId []byte, verify bool) (*Object, error) {

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
	if verify {
		//todo: generate and verify data to make sure the state matches what is in the patricia trie
	}
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
		//pull current state entry from the database.
		currentState.stateData, err = sdb.GetPersistentEntry(chainId, false)
		if err != nil {

			return nil, fmt.Errorf("no current state is defined, %v", err)
		}
	}

	return ret, nil
}

func (sdb *StateDB) getOrCreateChainMerkleManager(chainId []byte, loadState bool) *merkleManagerState {
	var mms *merkleManagerState
	var key managed.Hash
	copy(key[:], chainId)

	if mms = sdb.mms[key]; mms == nil {
		mms = new(merkleManagerState)
		//this will load the merkle state to the current state for the chainId
		mms.merkleMgr = sdb.mm.Copy(chainId)
		sdb.mms[key] = mms
	}
	if loadState {
		obj, _ := sdb.GetCurrentEntry(chainId)
		mms.stateEntry = *obj
	}
	return mms
}

// AddStateEntry add the entry to the smt and database based upon chainId
func (sdb *StateDB) AddStateEntry(chainId []byte, entry []byte) error {

	begin := time.Now()

	//mms := sdb.getOrCreateChainMerkleManager(chainId, false)

	sdb.TimeBucket = sdb.TimeBucket + float64(time.Since(begin))*float64(time.Nanosecond)*1e-9

	var key types.Bytes32
	copy(key[:], chainId)
	sdb.mutex.Lock()
	updates := sdb.updates[key]
	sdb.mutex.Unlock()

	if updates == nil {
		panic("cannot update states without validated transactions")
	}
	if updates.stateData == nil {
		updates.stateData = new(Object)
	}

	updates.stateData.Entry = entry

	return nil
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
	keys := make([]types.Bytes32, 0, currentStateCount)
	for chainId := range sdb.updates {
		keys = append(keys, chainId)
	}

	//sort the keys
	sort.Slice(keys, func(i, j int) bool {
		return bytes.Compare(keys[i][:], keys[j][:]) < 0
	})

	//then run through the list and record them
	//loop through everything and write out states to the database.
	for _, chainId := range keys {
		v := sdb.mm.Copy(chainId[:])
		// We get ChainState objects here, instead. And THAT will hold
		//       the MerkleStateManager for the chain.
		currentState := sdb.updates[chainId]
		if currentState == nil {
			panic(fmt.Sprintf("Chain state is nil meaning no updates were stored on chain %X for the block. Should not get here!", chainId[:]))
		}

		//add all the validated state changes that occured during this block (in order of appearance)
		for _, tx := range currentState.validatedTx {
			data, _ := tx.Transaction.MarshalBinary()

			//transaction hash is already computed so should match sha256(data)
			v.AddHash(managed.Hash(*tx.transactionHash))

			//store the transaction
			v.RootDBManager.PutBatch("Tx", "", tx.transactionHash.Bytes(), data)
		}

		//add all the pending transaction states that occurred during this block (in order of appearance)
		for _, tx := range currentState.pendingTx {
			//marshal the pending transaction state
			data, _ := tx.MarshalBinary()

			//hash it and add to the merkle state for the pending chain
			pendingHash := sha256.Sum256(data)
			v.AddPendingHash(pendingHash)

			//Store the mapping of the Transaction hash to the pending transaction hash which can be used for validation so we can find
			//the pending transaction
			v.RootDBManager.PutBatch("MainToPending", "", tx.TransactionState.transactionHash.Bytes(), pendingHash[:])

			//store the pending transaction by the pending tx hash
			v.RootDBManager.PutBatch("PendingTx", "", pendingHash[:], data)
		}

		if len(currentState.validatedTx) != 0 {
			mdRoot := v.MainChain.MS.GetMDRoot()
			if mdRoot == nil {
				//shouldn't get here, but will reject if I do
				panic(fmt.Sprintf("shouldn't get here on writeState() on chain id %X obtaining merkle state", chainId))
			}

			//store the state of the main chain in the state object
			currentState.stateData.MDRoot = mdRoot[:]

			//now store the state object
			chainStateObject, err := currentState.stateData.MarshalBinary()
			err = sdb.GetDB().PutBatch("StateEntries", "", chainId.Bytes(), chainStateObject)

			if err != nil {
				//shouldn't get here, and bad if I do...
				panic(fmt.Sprintf("failed to store data entry in StateEntries bucket, %v", err))
			}
			// The bpt stores the hash of the ChainState object hash.
			sdb.bpt.Bpt.Insert(chainId, sha256.Sum256(chainStateObject))
		}

		if len(currentState.pendingTx) != 0 {
			mdRoot := v.PendingChain.MS.GetMDRoot()
			if mdRoot == nil {
				//shouldn't get here, but will reject if I do
				panic(fmt.Sprintf("shouldn't get here on writeState() on chain id %X obtaining merkle state", chainId))
			}
			//todo:  Determine if we need to store the mdroot of the pending chain in the BPT
			sdb.bpt.Bpt.Insert(chainId, *mdRoot)
		}
	}

	sdb.bpt.Bpt.Update()
	sdb.mm.RootDBManager.EndBatch()

	//reset out block update buffer to get ready for the next round
	sdb.updates = make(map[types.Bytes32]*blockUpdates)

	//return the state of the BPT for the state of the block
	return sdb.bpt.Bpt.Root.Hash[:], currentStateCount, nil
}
