package state

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"sort"

	"github.com/AccumulateNetwork/SMT/managed"
	"github.com/AccumulateNetwork/SMT/pmt"
	smtdb "github.com/AccumulateNetwork/SMT/storage/database"
)

type merkleManagerState struct {
	merkleMgr  *managed.MerkleManager //merkleMgr manages the merkle state for the chain
	stateEntry Object                 //stateEntry is the latest state entry for the current height
}

type pendingTxState struct {
	tx []byte
	//do we need to store something else here?
}

type pendingValidationState struct {
	txid   []byte // hash of the transaction, can be validated
	pubkey []byte // public key used to sign txid
	sig    []byte // signature of pending validation, this is keyed by sha256(pendingTxState.tx)
}

// StateDB the state DB will only retrieve information out of the database.  To store stuff use PersistentStateDB instead
type StateDB struct {
	db        *smtdb.Manager
	debug     bool
	mms       map[managed.Hash]*merkleManagerState
	pendingTx map[managed.Hash]*pendingTxState
	bpt       *pmt.Manager
}

// Open database to manage the smt and chain states
func (sdb *StateDB) Open(dbfilename string, usememdb bool, debug bool) error {
	dbtype := "badger"
	if usememdb {
		dbtype = "memory"
	}

	sdb.db = &smtdb.Manager{}
	err := sdb.db.Init(dbtype, dbfilename)
	if err != nil {
		return err
	}

	sdb.db.AddBucket("StateEntries")
	sdb.db.AddBucket("PendingEntries")
	sdb.db.AddBucket("PendingTx")
	sdb.db.AddBucket("PendingSig")
	sdb.debug = debug
	if debug {
		sdb.db.AddBucket("Entries-Debug") //items will bet pushed into this bucket as the state entries change
	}

	sdb.mms = make(map[managed.Hash]*merkleManagerState)
	sdb.pendingTx = make(map[managed.Hash]*pendingTxState)
	sdb.bpt = pmt.NewBPTManager(sdb.db)
	return nil
}

func (sdb *StateDB) GetDB() *smtdb.Manager {
	return sdb.db
}

//AddPendingTx adds the pending tx raw data and signature of that data to tx, signature needs to be a signed hash of the tx.
func (sdb *StateDB) AddPendingTx(chainId []byte, txRaw []byte, sig []byte) error {
	//
	//var key managed.Hash
	//copy(key[:], chainId)
	//if tx = sdb.pendingTx[key]; tx == nil {
	//	tx = new(pendingTxState)
	//	mms.merkleMgr = managed.NewMerkleManager(sdb.db, chainId, 8)
	//	sdb.mms[key] = mms
	//}
	//
	//////The Entry is the State object derived from the transaction
	//mms.stateEntry = txRaw
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

//GetCurrentState retrieves the current state object from the database based upon chainid.  Current state either comes
//from a previously saves state for the current block, or it is from the database
func (sdb *StateDB) GetCurrentEntry(chainId []byte) (*Object, error) {
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

// AddStateEntry add the entry to the smt and database based upon chainid
func (sdb *StateDB) AddStateEntry(chainId []byte, entry []byte) error {
	var mms *merkleManagerState

	var key managed.Hash
	copy(key[:], chainId)
	//note: keys will be added to the map, but a map won't store them in order added.
	//this is ok since the chains are independent of one another.  The BPT will look
	//the same no matter what order the chains are added for a particular block.
	if mms = sdb.mms[key]; mms == nil {
		mms = new(merkleManagerState)
		mms.merkleMgr = managed.NewMerkleManager(sdb.db, chainId, 8)
		sdb.mms[key] = mms
	}

	////The Entry is the State object derived from the transaction
	mms.stateEntry.Entry = entry
	return nil
}

// writeStates will push the data to the database and update the patricia trie
func (sdb *StateDB) WriteStates(blockHeight int64) ([]byte, error) {
	//build a list of keys from the map
	keys := make([]managed.Hash, 0, len(sdb.mms))
	for chainId, _ := range sdb.mms {
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
		hash := sha256.Sum256(v.stateEntry.Entry)

		v.merkleMgr.SetBlockIndex(blockHeight)

		v.merkleMgr.AddHash(hash)
		v.stateEntry.StateIndex = v.merkleMgr.GetElementCount()
		mdroot := v.merkleMgr.MainChain.MS.GetMDRoot()
		if mdroot == nil {
			//shouldn't get here, but will reject if I do
			return nil, fmt.Errorf("shouldn't get here on writeState() on chain id %X obtaining merkle state", chainId)
		}

		sdb.bpt.Bpt.Insert(chainId, *mdroot)

		//store the current state for the chain
		dataToStore, err := v.stateEntry.MarshalBinary()
		err = v.merkleMgr.RootDBManager.Put("StateEntries", "", chainId.Bytes(), dataToStore)

		if err != nil {
			return nil, fmt.Errorf("failed to store data entry in StateEntries bucket, %v", err)
		}
	}
	sdb.bpt.Bpt.Update()

	sdb.mms = make(map[managed.Hash]*merkleManagerState)
	sdb.pendingTx = make(map[managed.Hash]*pendingTxState)

	return sdb.bpt.Bpt.Root.Hash[:], nil
}
