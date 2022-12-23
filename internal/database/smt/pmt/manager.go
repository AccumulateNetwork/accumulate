// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pmt

import (
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/pmt/model"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage/memory"
)

type Manager struct {
	// DBManager storage.KeyValueTxn
	model *model.BPT
	Dirty []*BptNode
	Bpt   *BPT
}

// NewBPTManager
// Get a new BPTManager which keeps the BPT on disk.  If the BPT is on
// disk, then it can be reloaded as needed.
func NewBPTManager(dbManager storage.KeyValueTxn) *Manager { // Return a new BPTManager
	if dbManager == nil { //                       If no dbManager is provided,
		store := memory.New(nil)      //           Create a memory one
		dbManager = store.Begin(true) //
	}

	store := record.KvStore{Store: dbManager}
	return New(nil, store, record.Key{"BPT"}, "bpt")
}

// New creates a new BPT manager for the given key.
func New(logger log.Logger, store record.Store, key record.Key, label string) *Manager { // Return a new BPTManager
	model := model.New(logger, store, key, label)
	manager := new(Manager)       //               Allocate the struct
	manager.model = model         //               populate with pointer to the database manager
	manager.Bpt = NewBPT(manager) //               Allocate a new BPT
	manager.Bpt.Manager = manager //               Allow the Bpt to call back to the manager for db access
	s, e := model.State().Get()   //               Get the BPT settings from disk
	if e == nil {                 //               If nothing is found, well this is a fresh instance
		manager.Bpt.load(s)                     // But if data is found, then unmarshal
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

	data, e := m.model.Block(node.NodeKey).Get() //                      Get the Byte Block
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
		data := m.Bpt.MarshalByteBlock(node)         //
		err := m.model.Block(node.NodeKey).Put(data) //
		if err != nil {
			return err
		}
		if node.Height == 0 {
			err = m.model.State().Put(&m.Bpt.RootState)
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
