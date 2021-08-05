package state

import "fmt"

type PersistentStateDB struct {
	StateDB
}

//Returns state hash of marshalled data
func (sdb *PersistentStateDB) PutStateObject(chainid []byte, so *StateObject) error {
	if so == nil {
		return fmt.Errorf("Cannot State Object is nil")
	}
	data, err := so.Marshal()
	if err != nil {
		return fmt.Errorf("Cannot Marshal State Object %v", err)
	}
	//
	err = sdb.db.Put("StateEntries", "", chainid, data)
	if err != nil {
		return err
	}
	//if debugenabled == true {
	//}
	//if data != nil {
	//	ret = &StateObject{}
	//	err = ret.Unmarshal(data)
	//	if err != nil {
	//		return nil, fmt.Errorf("No Current State is Defined")
	//	}
	//}
	return nil
}

//
//func (sdb *PersistentStateDB) PutStateEntryDebug(statehash []byte) (ret []byte, err error) {
//	data := sdb.db.Get("Entries-Debug", "", statehash)
//	if data != nil {
//		return nil, fmt.Errorf("No entry found for state hash %v", statehash)
//	}
//	return ret, nil
//}
