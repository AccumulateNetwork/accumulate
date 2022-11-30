// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pmt

// Entry
// We only have two node types, a Node that builds the Patricia Tree, and
// a Value that holds the values at the leaves.
type Entry interface {
	T() int                       // Returns the type of entry
	GetHash() []byte              // Returns the Hash for the entry
	Marshal() []byte              // Serialize the state of the Node or Value
	UnMarshal(data []byte) []byte // Unmarshal the state into the Node or Value
	Equal(entry Entry) bool       // Return Entry == entry
}

const (
	TNil       = iota + 1 // When persisting, this is the type for nils
	TNode                 // Type for Nodes
	TValue                // Type for values
	TNotLoaded            // When transisioning into a new Byte Block, the NotLoaded indicates a need to load from disk
)
