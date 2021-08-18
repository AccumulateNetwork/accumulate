package accnode

import (
	"fmt"
	smtdb "github.com/AccumulateNetwork/SMT/storage/database"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

// StateDB the state DB will only retrieve information out of the database.  To store stuff use PersistentStateDB instead
type StateDB struct {
	db    *smtdb.Manager
	debug bool
}

// Open database to manage the smt and chain states
func (sdb *StateDB) Open(path string, usememdb bool, debug bool) error {
	dbfilename := path + "/" + "valacc.db"
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
	return nil
}

func (sdb *StateDB) GetDB() *smtdb.Manager {
	return sdb.db
}

func (sdb *StateDB) GetStateObject(chainid []byte, verify bool) (ret *state.Object, err error) {
	if sdb.db == nil {
		return nil, fmt.Errorf("database has not been initialized")
	}
	data := sdb.db.Get("StateEntries", "", chainid)
	if data != nil {
		ret = &state.Object{}
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
