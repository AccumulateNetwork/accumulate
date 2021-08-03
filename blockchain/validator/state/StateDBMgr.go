package state

import "fmt"

type StateDBMgr struct {
	dba []*StateDB
}

func NewStateDBMgr(numnetworks int) *StateDBMgr {
	dbmgr := &StateDBMgr{}
	//create
	dbmgr.dba = make([]*StateDB, numnetworks)
	return dbmgr
}

func (s *StateDBMgr) AddStateDB(networkid int, db *StateDB) error {
	if networkid < 0 || networkid > len(s.dba) {
		return fmt.Errorf("Network address out of range %d expected < %d", networkid, len(s.dba))
	}
	s.dba[networkid] = db
	return nil
}

func (s *StateDBMgr) GetStateDB(networkid int) *StateDB {
	if networkid < 0 || networkid > len(s.dba) {
		return nil
	}
	return s.dba[networkid]
}

func (s *StateDBMgr) Verify() error {
	for i, sdb := range s.dba {
		if sdb == nil {
			return fmt.Errorf("StateDBMgr database missing for network %d", i)
		}
	}
	return nil
}
