// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"bytes"
)

// walkRange
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
func (b *BPT) walkRange(found *bool, n *branch, count int, key [32]byte, values []*leaf) []*leaf {
	// Function to process the entry (which is both left and right, but the same logic)
	do := func(entry node) {
		switch e := entry.(type) { //
		case *emptyNode: //                                       If we find a nil, we have found our starting
			*found = true //                                        point. But we have nothing to do.
			return        //                                               we are done.
		case *branch: //                               If a node, recurse and get its stuff.
			values = b.walkRange(found, e, count, key, values) // **Note values is updated here only
		case *leaf: //                                                 If not a node, not nil, it is a value.
			v := e                             //                   Get the value
			*found = true                      //                   No matter what, start collecting the range
			if bytes.Equal(key[:], v.Key[:]) { //                   But don't collect if equal to the key
				return //                                             because we use the last key to get the
			} //                                                      next range
			value := new(leaf)             //                       Otherwise copy the value out of the BPT
			value.Key = v.Key              //                       and stuff it in the values list
			value.Hash = v.Hash            //
			values = append(values, value) //
			return                         //
		}
	}

	// Need to walk down the bits of the key, to load data into memory, and to find the starting point
	BIdx := byte(n.Height >> 3) //     Calculate the byte index based on the height of this node in the BPT
	bitIdx := n.Height & 7      //     The bit index is given by the lower 3 bits of the height
	bit := byte(0x80) >> bitIdx //     The mask starts at the high end bit in the byte, shifted right by the bitIdx

	// Load the node and hope it doesn't fail
	_ = n.load()

	if len(values) >= count { //          See if done
		return values //                     and leave if done
	}

	if *found || key[BIdx]&bit > 0 { //   If the start key is found, or going left tracks the start key
		do(n.Left) //        then look Left to find the start key/collect range values
	}

	if len(values) >= count { //          See if done
		return values //                and leave if done
	}

	do(n.Right) //                     look to the right

	return values //                      return the values found so far
}
