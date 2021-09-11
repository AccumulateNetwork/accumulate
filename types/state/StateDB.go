package state

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"sort"

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
	tx []byte
	//do we need to store something else here?
}

type pendingValidationState struct {
	pendingTx *pendingTxState
	txId      []byte // hash of the transaction, can be validated
	pubKey    []byte // public key used to sign txId
	sig       []byte // signature of pending validation, this is keyed by sha256(pendingTxState.tx)
}

// StateDB the state DB will only retrieve information out of the database.  To store stuff use PersistentStateDB instead
type StateDB struct {
	db    *smtDB.Manager
	debug bool
	mms   map[managed.Hash]*merkleManagerState //mms is the merkle manager state map cached for the block, it is reset after each call to WriteState
	bpt   *pmt.Manager                         //pbt is the global patricia trie for the application
	mm    *managed.MerkleManager               //mm is the merkle manager for the application.  The salt is set by the appId
	appId []byte
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
	sdb.db.AddBucket("PendingEntries")
	sdb.db.AddBucket("PendingTx")
	sdb.debug = debug
	if debug {
		sdb.db.AddBucket("Entries-Debug") //items will bet pushed into this bucket as the state entries change
	}

	sdb.mms = make(map[managed.Hash]*merkleManagerState)

	sdb.bpt = pmt.NewBPTManager(sdb.db)
	sdb.mm = managed.NewMerkleManager(sdb.db, appId, markPower)
	sdb.appId = appId
	return nil
}

func (sdb *StateDB) GetDB() *smtDB.Manager {
	return sdb.db
}

//AddPendingTx adds the pending tx raw data and signature of that data to tx, signature needs to be a signed hash of the tx.
func (sdb *StateDB) AddPendingTx(chainId []byte, txRaw []byte, sig []byte, pubKey []byte) error {

	mms := sdb.getOrCreateChainMerkleManager(chainId, false)
	txId := sha256.Sum256(txRaw)

	mms.merkleMgr.AddPendingHash(txId)

	pvs := &pendingValidationState{}
	pvs.txId = txId[:]

	ptx := &pendingTxState{}
	ptx.tx = txRaw

	pvs.pendingTx = ptx
	pvs.sig = sig
	pvs.pubKey = pubKey

	mms.pending = append(mms.pending, pvs)

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
		//todo: generate and verify data the receipts to make sure the information is valid
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
	var key managed.Hash
	key.Extract(chainId)
	if mms := sdb.mms[key]; mms != nil {
		ret = &mms.stateEntry
	} else {
		//pull current state from the database.
		ret, err = sdb.GetPersistentEntry(chainId, false)
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

	mms := sdb.getOrCreateChainMerkleManager(chainId, false)
	//now load the state
	mms.merkleMgr.AddHash(sha256.Sum256(entry))

	////The Entry is the State object derived from the transaction
	mms.stateEntry.Entry = entry

	return nil
}

// WriteStates will push the data to the database and update the patricia trie
func (sdb *StateDB) WriteStates(blockHeight int64) ([]byte, error) {
	//build a list of keys from the map

	currentStateCount := len(sdb.mms)
	if currentStateCount > 0 {
		//only attempt to record the block if we have any data.
		sdb.mm.SetBlockIndex(blockHeight)
	}
	keys := make([]managed.Hash, 0, currentStateCount)
	for chainId := range sdb.mms {
		keys = append(keys, chainId)
	}

	//sort the keys
	sort.Slice(keys, func(i, j int) bool {
		return bytes.Compare(keys[i][:], keys[j][:]) < 0
	})

	//then run through the list and record them
	//loop through everything and write out states to the database.
	for _, chainId := range keys {
		v := sdb.mms[chainId]
		//hash := sha256.Sum256(v.stateEntry.Entry)
		//
		//v.merkleMgr.AddHash(hash)

		v.stateEntry.StateIndex = v.merkleMgr.GetElementCount()
		mdRoot := v.merkleMgr.MainChain.MS.GetMDRoot()
		if mdRoot == nil {
			//shouldn't get here, but will reject if I do
			return nil, fmt.Errorf("shouldn't get here on writeState() on chain id %X obtaining merkle state", chainId)
		}

		sdb.bpt.Bpt.Insert(chainId, *mdRoot)

		//store the current state for the chain
		dataToStore, err := v.stateEntry.MarshalBinary()
		err = v.merkleMgr.RootDBManager.Put("StateEntries", "", chainId.Bytes(), dataToStore)

		if err != nil {
			return nil, fmt.Errorf("failed to store data entry in StateEntries bucket, %v", err)
		}
	}

	sdb.bpt.Bpt.Update()

	sdb.mms = make(map[managed.Hash]*merkleManagerState)

	return sdb.bpt.Bpt.Root.Hash[:], nil
}
