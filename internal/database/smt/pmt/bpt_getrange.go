// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pmt

import (
	"bytes"
	"fmt"
)

// Print
// Print the BPT.  Only works if the whole BPT is in memory.
//nolint:noprint
func (b *BPT) Print(entry Entry) {

	switch { //                                                 processing is done once here.
	case entry == nil: //                                       Sort if the Left/Right is nil.
		return //                                               we are done.
	case (entry).T() == TNode: //                               If the entry isn't nil, check if it is a Node
		node := entry.(*BptNode)
		fmt.Print("L ")
		b.Print(node.Left)
		for i := 0; i < node.Height; i++ {
			fmt.Print("  ")
		}
		fmt.Print("R ")
		b.Print(node.Right)
	default: //
		value := entry.(*Value)
		fmt.Printf("key %03x value %03x\n", value.Key[:3], value.Hash[:3])
	}
}

// WalkRange
// A recursive routine that pushes collisions towards the leaves of the
// binary patricia tree until the keys don't match any more.  Note that
// this tree cannot handle duplicate keys, but that is an assumption of
// patricia trees anyway
//
// Inputs:
// node -- the node in the BPT where the value (key, hash) is being inserted
// count -- the number of hashes to collect
// key  -- The key in the BPT which determines were in the BPT the hash goes
// hash -- The current value of the key, as tracked by the BPT
func (b *BPT) WalkRange(found *bool, node *BptNode, count int, key [32]byte, values []*Value) []*Value {
	// Function to process the entry (which is both left and right, but the same logic)
	do := func(entry Entry) {
		switch { //
		case entry == nil: //                                       If we find a nil, we have found our starting
			*found = true //                                        point. But we have nothing to do.
			return        //                                               we are done.
		case (entry).T() == TNode: //                               If a node, recurse and get its stuff.
			values = b.WalkRange(found, (entry).(*BptNode), count, key, values) // **Note values is updated here only
		default: //                                                 If not a node, not nil, it is a value.
			v := (entry).(*Value)              //                   Get the value
			*found = true                      //                   No matter what, start collecting the range
			if bytes.Equal(key[:], v.Key[:]) { //                   But don't collect if equal to the key
				return //                                             because we use the last key to get the
			} //                                                      next range
			value := new(Value)            //                       Otherwise copy the value out of the BPT
			value.Key = v.Key              //                       and stuff it in the values list
			value.Hash = v.Hash            //
			values = append(values, value) //
			return                         //
		}
	}

	// Need to walk down the bits of the key, to load data into memory, and to find the starting point
	BIdx := byte(node.Height >> 3) //     Calculate the byte index based on the height of this node in the BPT
	bitIdx := node.Height & 7      //     The bit index is given by the lower 3 bits of the height
	bit := byte(0x80) >> bitIdx    //     The mask starts at the high end bit in the byte, shifted right by the bitIdx

	b.LoadNext(BIdx, bit, node, key) //   We also need the BIdx and bit to make sure the BPT is loaded.

	if len(values) >= count { //          See if done
		return values //                     and leave if done
	}

	if *found || key[BIdx]&bit > 0 { //   If the start key is found, or going left tracks the start key
		do(node.Left) //        then look Left to find the start key/collect range values
	}

	if len(values) >= count { //          See if done
		return values //                and leave if done
	}

	do(node.Right) //                     look to the right

	return values //                      return the values found so far
}

// Insert
// Starts the search of the BPT for the location of the key in the BPT
func (b *BPT) GetRange(startKey [32]byte, count int) (values []*Value, lastKey [32]byte) {
	if count == 0 { // If they didn't ask for anything, there is nothing to do.
		return
	}
	var found bool                                                     // We use found as flag as a solid state that we found our start
	values = b.WalkRange(&found, b.GetRoot(), count, startKey, values) // Look for the starting point, and collect a "count" number of entries
	if len(values) > 0 {                                               // If we got something, go ahead and return the last element
		lastKey = values[len(values)-1].Key //                             The lastKey can be easily used to ask for another contiguous range
	}
	return values, lastKey
} //
