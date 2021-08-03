package state

import (
	"fmt"
	smtdb "github.com/AccumulateNetwork/SMT/storage/database"
)

type StateDB struct {
	db smtdb.Manager
}

func (sdb *StateDB) Open(path string) {
	dbfilename := path + "/" + "valacc.db"
	dbtype := "badger"
	//dbtype := "memory" ////for kicks just create an in-memory database for now
	sdb.db.Init(dbtype, dbfilename)

	sdb.db.AddBucket("Entries-Debug") //items will bet pushed into this bucket as the state entries change
	sdb.db.AddBucket("StateEntries")
}

func (sdb *StateDB) GetDB() *smtdb.Manager {
	return &sdb.db
}

func (sdb *StateDB) GetStateObject(chainid []byte) (ret *StateObject, err error) {
	data := sdb.db.Get("StateEntries", "", chainid)
	if data != nil {
		ret = &StateObject{}
		err = ret.Unmarshal(data)
		if err != nil {
			return nil, fmt.Errorf("No Current State is Defined")
		}
	}
	return ret, nil
}

func (sdb *StateDB) GetStateEntryDebug(statehash []byte) (ret *StateEntry, err error) {
	data := sdb.db.Get("Entries-Debug", "", statehash)
}
