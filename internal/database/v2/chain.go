package database

import (
	"fmt"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
)

const markPower = 8
const markFreq = 1 << markPower
const markMask = markFreq - 1

func newChain(store recordStore, key recordKey, typ protocol.ChainType, namefmt, labelfmt string) *Chain {
	c := new(Chain)
	c.store = store
	c.key = key
	c.typ = typ
	if strings.ContainsRune(namefmt, '%') {
		c.name = fmt.Sprintf(namefmt, key...)
	} else {
		c.name = namefmt
	}
	c.label = fmt.Sprintf(labelfmt, key...)
	return c
}

func newMajorMinorIndexChain(store recordStore, key recordKey, namefmt, labelfmt string) *MajorMinorIndexChain {
	c := new(MajorMinorIndexChain)
	c.store = store
	c.key = key
	if strings.ContainsRune(namefmt, '%') {
		c.name = fmt.Sprintf(namefmt, key...)
	} else {
		c.name = namefmt
	}
	c.label = fmt.Sprintf(labelfmt, key...)
	return c
}

func (c *Chain) AddHash(hash managed.Hash, unique bool) error {
	state, err := c.State().Get() // Get the current state
	if err != nil {
		return err
	}

	hash = hash.Copy()                         // Just to make sure hash doesn't get changed
	elemIdx := c.ElementIndex(hash)            //
	_, err = elemIdx.Get()                     // See if this element is a duplicate
	if errors.Is(err, errors.StatusNotFound) { // So only if the hash is not yet added to the Merkle Tree
		err = elemIdx.Put(uint64(state.Count)) // Keep its index
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if unique {
		return nil // Don't add duplicates
	}

	err = c.Element(uint64(state.Count)).Put(hash)
	if err != nil {
		return err
	}
	switch (state.Count + 1) & markMask {
	case 0: // Is this the end of the Mark set, i.e. 0, ..., m.MarkFreq-1
		state.AddToMerkleTree(hash)                        // Add the hash to the Merkle Tree
		err = c.States(uint64(state.Count - 1)).Put(state) // Save Merkle State at n*MarkFreq-1
		if err != nil {
			return err
		}
	case 1: //                              After MarkFreq elements are written
		state.HashList = state.HashList[:0] // then clear the HashList
		fallthrough                         // then fall through as normal
	default:
		state.AddToMerkleTree(hash) // 0 to m.MarkFeq-2, always add to the merkle tree
	}
	err = c.State().Put(state)
	if err != nil {
		return fmt.Errorf("error writing chain head: %v", err)
	}

	return nil
}

func (c *Chain) Entries(start int64, end int64) ([][]byte, error) {
	state, err := c.State().Get()
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}

	if end > state.Count {
		end = state.Count
	}

	if end < start {
		return nil, errors.New(errors.StatusBadRequest, "invalid range: start is greater than end")
	}

	// GetRange will not cross mark point boundaries, so we may need to call it
	// multiple times
	entries := make([][]byte, 0, end-start)
	for start < end {
		h, err := c.GetRange(start, end)
		if err != nil {
			return nil, err
		}

		for i := range h {
			entries = append(entries, h[i])
		}
		start += int64(len(h))
	}

	return entries, nil
}

func (c *Chain) GetRange(begin, end int64) (hashes []managed.Hash, err error) {
	state, err := c.State().Get()
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}
	ec := state.Count

	// end++  Increment to include end in results, comment out to leave it out.

	if end < begin || begin >= ec || begin < 0 {
		return nil, fmt.Errorf("impossible range %d,%d for chain length %d",
			begin, end, ec) // Return zero begin and/or end are impossible
	}
	if end > ec { // Don't try and return more elements than are in the chain
		end = ec
	}
	if end == begin { // We will return an empty string if begin == end
		return hashes, nil
	}

	markPoint := begin & ^markMask // Get the mark point just past begin

	var s *managed.MerkleState
	var hl []managed.Hash // Collect all the hashes of the mark points covering the range of begin-end
	marks := (end-(begin&^markMask))/markFreq + 1
	for i := int64(0); i < marks; i++ {
		markPoint += markFreq
		if s, err = c.States(uint64(markPoint - 1)).Get(); err == nil {
			hl = append(hl, s.HashList...)
		} else {
			s, err = c.State().Get()
			if err != nil {
				return nil, errors.New(errors.StatusInternalError, "a chain should always have a chain state")
			}
			hl = append(hl, s.HashList...)
			break
		}
	}

	first := (begin) & markMask // Calculate the offset to the beginning of the range
	last := first + end - begin // and to the end of the range

	// FIXME Is this supposed to be an error?
	// if int(last) > len(hl) {
	// 	fmt.Println("begin end", begin, " ", end)
	// }
	return hl[first:last], nil // Return this slice.
}
