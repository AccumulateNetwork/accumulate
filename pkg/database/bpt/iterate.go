// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// ForEach calls the callback for each BPT entry.
func ForEach(b *BPT, fn func(key record.KeyHash, hash [32]byte) error) error {
	it := b.Iterate(1000)
	for it.Next() {
		for _, v := range it.Value() {
			err := fn(v.Key, v.Value)
			if err != nil {
				return err
			}
		}
	}
	return errors.UnknownError.Wrap(it.Err())
}

// Iterator iterates over the BPT.
type Iterator struct {
	bpt    *BPT
	place  [32]byte
	values []KeyValuePair
	last   []KeyValuePair
	err    error
}

// Iterate returns an iterator that iterates over the BPT, reading N entries at
// a time where N = window. Iterate panics if window is negative or zero.
func (b *BPT) Iterate(window int) *Iterator {
	if window <= 0 {
		panic("invalid window size")
	}

	it := new(Iterator)
	it.bpt = b
	it.values = make([]KeyValuePair, window)

	// This is the first possible BPT key.
	it.place = [32]byte{
		255, 255, 255, 255, 255, 255, 255, 255,
		255, 255, 255, 255, 255, 255, 255, 255,
		255, 255, 255, 255, 255, 255, 255, 255,
		255, 255, 255, 255, 255, 255, 255, 255,
	}
	return it
}

func (it *Iterator) Value() []KeyValuePair { return it.last }

// Next returns the next N results. Next returns false if there are no more
// results or if an error occurs. The caller must return Err after Next returns
// false to check for an error.
func (it *Iterator) Next() bool {
	if it.err != nil {
		return false
	}

	next, vals, err := it.bpt.getRange(it.place, it.values)
	if err != nil {
		it.err = errors.UnknownError.Wrap(err)
		return false
	}

	it.place = next
	it.last = vals
	return len(vals) > 0
}

// Err returns the error if one has occurred.
func (it *Iterator) Err() error { return it.err }

// walkNode processes the entry (which is both left and right, but the same logic)
func walkNode(n node, found *bool, key [32]byte, values []KeyValuePair, pos *int) error {
	switch n := n.(type) {
	case *emptyNode: //                                           If we find a nil, we have found our starting
		*found = true //                                          point. But we have nothing to do.

	case *branch: //                                              If a node, recurse and get its stuff.
		return n.walkRange(found, key, values, pos)

	case *leaf: //                                                If not a node, not nil, it is a value.
		*found = true            //                                      No matter what, start collecting the range
		if n.Key.Hash() == key { //                                      But don't collect if equal to the key
			break //                                              because we use the last key to get the
		} //                                                      next range
		values[*pos] = KeyValuePair{Key: n.Key.Hash(), Value: n.Hash} // Otherwise copy the value out of the BPT
		*pos++                                                        // and stuff it in the values list
	}

	return nil
}

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
func (b *branch) walkRange(found *bool, key [32]byte, values []KeyValuePair, pos *int) error {
	//                                                     Need to walk down the bits of the key, to load data into memory, and to find the starting point
	BIdx := byte(b.Height >> 3) //                         Calculate the byte index based on the height of this node in the BPT
	bitIdx := b.Height & 7      //                         The bit index is given by the lower 3 bits of the height
	bit := byte(0x80) >> bitIdx //                         The mask starts at the high end bit in the byte, shifted right by the bitIdx

	err := b.load() //                                     Load the node
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	if *pos >= len(values) { //                            See if done
		return nil //                                      and leave if done
	}

	if *found || key[BIdx]&bit > 0 { //                    If the start key is found, or going left tracks the start key
		err = walkNode(b.Left, found, key, values, pos) // then look Left to find the start key/collect range values
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	if *pos >= len(values) { //                            See if done
		return nil //                                      and leave if done
	}

	err = walkNode(b.Right, found, key, values, pos) //    look to the right
	return errors.UnknownError.Wrap(err)
}

func (b *BPT) getRange(startKey [32]byte, values []KeyValuePair) (lastKey [32]byte, _ []KeyValuePair, err error) {
	err = b.executePending() //                                    Execute any pending inserts
	if err != nil {
		return startKey, nil, errors.UnknownError.Wrap(err)
	}

	if len(values) == 0 {
		return startKey, nil, nil
	}

	var found bool                                              // We use found as flag as a solid state that we found our start
	var pos int                                                 //
	err = b.getRoot().walkRange(&found, startKey, values, &pos) // Look for the starting point, and collect a "count" number of entries
	if err != nil {
		return startKey, nil, errors.UnknownError.Wrap(err)
	}
	values = values[:pos]
	if len(values) == 0 {
		return startKey, values, nil
	}
	//                                                             If we got something, go ahead and return the last element
	return values[len(values)-1].Key, values, nil //               The lastKey can be easily used to ask for another contiguous range
}
