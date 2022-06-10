package pmt

import (
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/memory"
)

var kBpt = storage.MakeKey("BPT")
var kBptRoot = kBpt.Append("Root")

type Manager struct {
	DBManager storage.KeyValueTxn
	Dirty     []*BptNode
	Bpt       *BPT
}

// NewBPTManager
// Get a new BPTManager which keeps the BPT on disk.  If the BPT is on
// disk, then it can be reloaded as needed.
func NewBPTManager(dbManager storage.KeyValueTxn) *Manager { // Return a new BPTManager
	if dbManager == nil { //                       If no dbManager is provided,
		store := new(memory.DB)       //           Create a memory one
		dbManager = store.Begin(true) //
	}
	manager := new(Manager)            //          Allocate the struct
	manager.DBManager = dbManager      //          populate with pointer to the database manager
	manager.Bpt = NewBPT(manager)      //          Allocate a new BPT
	manager.Bpt.Manager = manager      //          Allow the Bpt to call back to the manager for db access
	data, e := dbManager.Get(kBptRoot) //          Get the BPT settings from disk
	if e == nil {                      //          If nothing is found, well this is a fresh instance
		manager.Bpt.UnMarshal(data)             // But if data is found, then unmarshal
		manager.LoadNode(manager.Bpt.GetRoot()) // and load up the root data for the BPT
	} //
	return manager //                              Return a new BPT manager
}

// GetRootHash
// Note this provides the root hash (generally) of the previous hash of
// the entire BPT.  If called JUST after a call to Update() then it returns
// the current root hash, which would be true up to the point that any
// node in the BPT is updated.
func (m *Manager) GetRootHash() [32]byte {
	return m.Bpt.RootHash
}

// LoadNode
// Loads the nodes under the given node into the BPT
func (m *Manager) LoadNode(node *BptNode) {
	if node.Height&m.Bpt.Mask != 0 { //                                           Throw an error if not a border node
		panic("load should not be called on a node that is not a border node") // panic -- should not occur
	}

	data, e := m.DBManager.Get(kBpt.Append(node.NodeKey[:])) //                      Get the Byte Block
	if e != nil {
		panic("Should have a Byte Block for any persisted BPT")
	}
	m.Bpt.UnMarshalByteBlock(node, data) //                      unpack it
	GetNodeHash(node)
}

// FlushNode
// Flushes the Byte Block to disk
func (m *Manager) FlushNode(node *BptNode) error { //   Flush a Byte Block
	if node.Height&7 == 0 {
		data := m.Bpt.MarshalByteBlock(node)                       //
		err := m.DBManager.Put(kBpt.Append(node.NodeKey[:]), data) //
		if err != nil {
			return err
		}
		if node.Height == 0 {
			data = m.Bpt.Marshal()
			err = m.DBManager.Put(kBptRoot, data)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// InsertKV
// Insert Key Value into Bpt
func (m *Manager) InsertKV(key, value [32]byte) {
	m.Bpt.Insert(key, value)
}
