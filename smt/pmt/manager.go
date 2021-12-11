package pmt

import (
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/smt/storage/database"
)

var kBpt = storage.MakeKey("BPT")
var kBptRoot = kBpt.Append("Root")

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
	manager := new(Manager)                     //            Allocate the struct
	manager.DBManager = dbManager               //            populate with pointer to the database manager
	manager.Bpt = NewBPT()                      //            Allocate a new BPT
	manager.Bpt.manager = manager               //            Allow the Bpt to call back to the manager for db access
	manager.LoadedBB = make(map[[32]byte]*Node) //            Allocate an initial map
	data, e := dbManager.Get(kBptRoot)          //            Get the BPT settings from disk
	if e == nil {                               //            If nothing is found, well this is a fresh instance
		manager.Bpt.UnMarshal(data)        //                 But if data is found, then unmarshal
		manager.LoadNode(manager.Bpt.Root) //                 and load up the root data for the BPT
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
		data, e := m.DBManager.Get(kBpt.Append(node.BBKey[:])) //                      Get the Byte Block
		if e != nil {
			return nil
		}
		m.Bpt.UnMarshalByteBlock(node, data) //                      unpack it
		m.LoadedBB[node.BBKey] = node        //                      Save the root node of Byte Block
		return node
	} else {
		return n
	}
}

// FlushNode
// Flushes the Byte Block to disk
func (m *Manager) FlushNode(node *Node) { //   Flush a Byte Block
	data := m.Bpt.MarshalByteBlock(node)                   //
	m.DBManager.PutBatch(kBpt.Append(node.BBKey[:]), data) //
	if node.Height == 0 {
		data = m.Bpt.Marshal()
		m.DBManager.PutBatch(kBptRoot, data)
	}
}

// InsertKV
// Insert Key Value into Bpt
func (m *Manager) InsertKV(key, value [32]byte) {
	m.Bpt.Insert(key, value)
}
