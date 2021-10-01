package pmt

import (
	"github.com/AccumulateNetwork/accumulated/smt/storage/database"
)

type Manager struct {
	DBManager *database.Manager
	Dirty     []*Node
	Bpt       *BPT
	LoadedBB  map[[32]byte]*Node
}

// NewBPTManager
// Get a new BPTManager which keeps the BPT on disk.  If the BPT is on
// disk, then it can be reloaded as needed.
func NewBPTManager(dbManager *database.Manager) *Manager { // Return a new BPTManager
	manager := new(Manager)                        //         Allocate the struct
	manager.DBManager = dbManager                  //         populate with pointer to the database manager
	manager.Bpt = NewBPT()                         //         Allocate a new BPT
	manager.Bpt.manager = manager                  //         Allow the Bpt to call back to the manager for db access
	manager.LoadedBB = make(map[[32]byte]*Node)    //         Allocate an initial map
	data := dbManager.Get("BPT", "Root", []byte{}) //         Get the BPT settings from disk
	if data != nil {                               //         If nothing is found, well this is a fresh instance
		manager.Bpt.UnMarshal(data)        //                  But if data is found, then unmarshal
		manager.LoadNode(manager.Bpt.Root) //                  and load up the root data for the BPT
	} //
	return manager //                                         Return a new BPT manager
}

// GetRootHash
// Note this provides the root hash (generally) of the previous hash of
// the entire BPT.  If called JUST after a call to Update() then it returns
// the current root hash, which would be true up to the point that any
// node in the BPT is updated.
func (m *Manager) GetRootHash() [32]byte {
	return m.Bpt.Root.Hash
}

// LoadNode
// Loads the nodes under the given node into the BPT
func (m *Manager) LoadNode(node *Node) *Node {
	if node.Height&m.Bpt.mask != 0 { //                                           Throw an error if not a border node
		panic("load should not be called on a node that is not a border node") // panic -- should not occur
	}
	if n := m.LoadedBB[node.BBKey]; n == nil { //                                 If the Byte Block isn't loaded
		data := m.DBManager.Get("BPT", "", node.BBKey[:]) //                      Get the Byte Block
		m.Bpt.UnMarshalByteBlock(node, data)              //                      unpack it
		m.LoadedBB[node.BBKey] = node                     //                      Save the root node of Byte Block
		return node
	} else {
		return n
	}
}

// FlushNode
// Flushes the Byte Block to disk
func (m *Manager) FlushNode(node *Node) { //   Flush a Byte Block
	data := m.Bpt.MarshalByteBlock(node)                 //
	m.DBManager.PutBatch("BPT", "", node.BBKey[:], data) //
	if node.Height == 0 {
		data = m.Bpt.Marshal()
		m.DBManager.PutBatch("BPT", "Root", []byte{}, data)
	}
}

// InsertKV
// Insert Key Value into Bpt
func (m *Manager) InsertKV(key, value [32]byte) {
	m.Bpt.Insert(key, value)
}
