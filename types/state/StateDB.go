package state

import (
	"crypto/sha256"
	"fmt"
	"github.com/AccumulateNetwork/SMT/managed"
	"github.com/AccumulateNetwork/SMT/pmt"
	smtdb "github.com/AccumulateNetwork/SMT/storage/database"
)

type merkleManagerState struct {
	merklemgr          *managed.MerkleManager
	currentStateObject Object
	stateObjects       []Object //all the state objects for this height, if we don't care about history this can go byebye...
}

// StateDB the state DB will only retrieve information out of the database.  To store stuff use PersistentStateDB instead
type StateDB struct {
	db    *smtdb.Manager
	debug bool
	mms   map[managed.Hash]*merkleManagerState
	bpt   *pmt.Manager
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
	sdb.debug = debug
	if debug {
		sdb.db.AddBucket("Entries-Debug") //items will bet pushed into this bucket as the state entries change
	}

	sdb.mms = make(map[managed.Hash]*merkleManagerState)
	sdb.bpt = pmt.NewBPTManager(sdb.db)
	return nil
}

func (sdb *StateDB) GetDB() *smtdb.Manager {
	return sdb.db
}

func (sdb *StateDB) SetStateObject(object *Object) error {

	//object.Entry.
	return nil
}

func (sdb *StateDB) GetStateObject(chainid []byte, verify bool) (ret *Object, err error) {
	if sdb.db == nil {
		return nil, fmt.Errorf("database has not been initialized")
	}
	data := sdb.db.Get("StateEntries", "", chainid)
	if data != nil {
		ret = &Object{}
		err = ret.Unmarshal(data)
		if err != nil {
			return nil, fmt.Errorf("no current state is defined")
		}
	}
	if verify {
		//todo: generate and verify data the receipts to make sure the information is valid
	}
	return ret, nil
}

func (sdb *StateDB) GetStateEntryDebug(statehash []byte) (ret []byte, err error) {
	if !sdb.debug {
		return nil, fmt.Errorf("no debug information stored")
	}
	if sdb.db == nil {
		return nil, fmt.Errorf("database has not been initialized")
	}
	data := sdb.db.Get("Entries-Debug", "", statehash)
	if data != nil {
		return nil, fmt.Errorf("no entry found for state hash %v", statehash)
	}
	return ret, nil
}

//getCurrentState retrieve the current state object from the database based upon chainid
func (sdb *StateDB) GetCurrentState(chainid []byte) (*Object, error) {
	var ret *Object
	var key managed.Hash
	key.Extract(chainid)
	if mms := sdb.mms[key]; mms != nil {
		ret = &mms.currentStateObject
	} else {
		//pull current state from the database.
		data := sdb.db.Get("StateEntries", "", chainid)
		if data != nil {
			ret = &Object{}
			err := ret.Unmarshal(data)
			if err != nil {
				return nil, fmt.Errorf("no current state is defined")
			}

		}
	}
	return ret, nil
}

// AddStateEntry add the entry to the smt and database based upon chainid
func (sdb *StateDB) AddStateEntry(chainid []byte, entry []byte) error {
	var mms *merkleManagerState

	hash := sha256.Sum256(entry)
	var key managed.Hash
	copy(key[:], chainid)
	//note: keys will be added to the map, but a map won't store them in order added.
	//this is ok since the chains are independent of one another.  The BPT will look
	//the same no matter what order the chains are added for a particular block.
	if mms = sdb.mms[key]; mms == nil {
		mms = new(merkleManagerState)
		mms.merklemgr = managed.NewMerkleManager(sdb.db, chainid, 8)
		sdb.mms[key] = mms
	}
	data := sdb.db.Get("StateEntries", "", chainid)
	if data != nil {
		currso := Object{}
		mms.currentStateObject.PrevStateHash = currso.PrevStateHash
	}
	mms.merklemgr.AddHash(hash)
	mdroot := mms.merklemgr.MainChain.MS.GetMDRoot()

	//The Entry feeds the Entry Hash, and the Entry Hash feeds the State Hash
	//The MD Root is the current state
	mms.currentStateObject.StateHash = mdroot.Bytes()
	//The Entry hash is the hash of the state object being stored
	mms.currentStateObject.EntryHash = hash[:]
	//The Entry is the State object derived from the transaction
	mms.currentStateObject.Entry = entry

	//list of the state objects from the beginning of the block to the end, so don't know if this needs to be kept
	mms.stateObjects = append(mms.stateObjects, mms.currentStateObject)
	return nil
}

// writeStates will push the data to the database and update the patricia trie
func (sdb *StateDB) WriteStates() ([]byte, error) {
	//loop through everything and write out states to the database.
	for chainId, v := range sdb.mms {
		mdroot := v.merklemgr.MainChain.MS.GetMDRoot()
		if mdroot == nil {
			//shouldn't get here, but will reject if I do
			return nil, fmt.Errorf("shouldn't get here on writeState() on chain id %X obtaining merkle state", chainId)
		}

		sdb.bpt.Bpt.Insert(chainId, *mdroot)
		dataToStore, err := v.currentStateObject.Marshal()
		if err != nil {
			//need to log failure
			continue
		}
		//store the current state for the chain
		err = sdb.db.Put("StateEntries", "", chainId.Bytes(), dataToStore)

		if err != nil {
			return nil, fmt.Errorf("failed to store data entry in StateEntries bucket, %v", err)
		}

		//iterate over the state objects updated as part of the state change and push the data for debugging
		for i := range v.stateObjects {
			data, err := v.stateObjects[i].Marshal()
			if err != nil {
				//shouldn't get here, but will reject if I do
				fmt.Printf("shouldn't get here on writeState() on chain id %X for updated states", chainId)
				continue
			}

			if sdb.debug {
				///TBD : this is not needed since we are maintaining only current state and not all states
				//just keeping for debug history.
				err = sdb.db.Put("Entries-Debug", "", v.stateObjects[i].StateHash, data)
				if err != nil {
					return nil, fmt.Errorf("failed to add debug entries, %v", err)
				}
			}
		}
		//delete it from our list.
		delete(sdb.mms, chainId)
	}
	sdb.bpt.Bpt.Update()

	return sdb.bpt.Bpt.Root.Hash[:], nil
}
