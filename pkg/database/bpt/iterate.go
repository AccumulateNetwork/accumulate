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
func ForEach(b BPT, fn func(key record.KeyHash, hash [32]byte) error) error {
	it := b.Iterate(window)
	for {
		bptVals, ok := it.Next()
		if !ok {
			break
		}

		for _, v := range bptVals {
			err := fn(v.Key, v.Value)
			if err != nil {
				return err
			}
		}
	}
	return errors.UnknownError.Wrap(it.Err())
}

type iterator struct {
	bpt    *bpt
	place  [32]byte
	values []KeyValuePair
	err    error
}

func (b *bpt) Iterate(window int) Iterator {
	if window <= 0 {
		panic("invalid window size")
	}

	it := new(iterator)
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

func (it *iterator) Next() ([]KeyValuePair, bool) {
	if it.err != nil {
		return nil, false
	}

	next, vals, err := it.bpt.getRange(it.place, it.values)
	if err != nil {
		it.err = errors.UnknownError.Wrap(err)
		return nil, false
	}

	it.place = next
	return vals, len(vals) > 0
}

func (it *iterator) Err() error { return it.err }

func walkNode(n node, found *bool, key [32]byte, values []KeyValuePair, pos *int) error {
	switch n := n.(type) {
	case *emptyNode:
		*found = true

	case *branch:
		return n.walkRange(found, key, values, pos)

	case *leaf:
		*found = true
		if n.Key == key {
			break
		}
		values[*pos] = KeyValuePair{Key: n.Key, Value: n.Hash}
		*pos++
	}

	return nil
}

func (b *branch) walkRange(found *bool, key [32]byte, values []KeyValuePair, pos *int) error {
	// Need to walk down the bits of the key, to load data into memory, and to find the starting point
	BIdx := byte(b.Height >> 3) //     Calculate the byte index based on the height of this node in the BPT
	bitIdx := b.Height & 7      //     The bit index is given by the lower 3 bits of the height
	bit := byte(0x80) >> bitIdx //     The mask starts at the high end bit in the byte, shifted right by the bitIdx

	err := b.load() // Load the node
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	if *pos >= len(values) {
		return nil
	}

	if *found || key[BIdx]&bit > 0 {
		walkNode(b.Left, found, key, values, pos)
	}

	if *pos >= len(values) {
		return nil
	}

	walkNode(b.Right, found, key, values, pos)

	return nil
}

func (b *bpt) getRange(startKey [32]byte, values []KeyValuePair) (lastKey [32]byte, _ []KeyValuePair, err error) {
	err = b.executePending() // Execute any pending inserts
	if err != nil {
		return startKey, nil, errors.UnknownError.Wrap(err)
	}

	if len(values) == 0 {
		return startKey, nil, nil
	}

	var found bool
	var pos int
	err = b.getRoot().walkRange(&found, startKey, values, &pos)
	if err != nil {
		return startKey, nil, errors.UnknownError.Wrap(err)
	}
	values = values[:pos]
	if len(values) == 0 {
		return startKey, values, nil
	}
	return values[len(values)-1].Key, values, nil
}
