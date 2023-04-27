// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type Iterator interface {
	Next() ([]KeyValuePair, bool)
	Err() error
}

type KeyValuePair struct {
	Key, Value [32]byte
}

// New returns a new BPT.
func New(parent database.Record, logger log.Logger, store database.Store, key *database.Key, label string) *BPT {
	b := new(BPT)
	b.logger.Set(logger)
	b.store = store
	b.key = key
	b.label = label
	return b
}

// executePending pushes pending updates into the tree.
func (b *BPT) executePending() error {
	// Push the updates
	for k, v := range b.pending {
		_, err := b.getRoot().merge(&leaf{Key: k, Hash: v}, true)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	// Optimized by the compiler
	for k := range b.pending {
		delete(b.pending, k)
	}
	return nil
}

// GetRootHash returns the root hash of the BPT, loading nodes, executing
// pending updates, and recalculating hashes if necessary.
func (b *BPT) GetRootHash() ([32]byte, error) {
	// Execute pending updates
	err := b.executePending()
	if err != nil {
		return [32]byte{}, errors.UnknownError.Wrap(err)
	}

	// Ensure the root node is loaded
	r := b.getRoot()
	err = r.load()
	if err != nil {
		return [32]byte{}, errors.UnknownError.WithFormat("load root: %w", err)
	}

	// Return its hash
	return r.getHash(), nil
}

func (b *BPT) newState() values.Value[*parameters] {
	v := values.NewValue(b.logger.L, b.store, b.key.Append("Root"), b.label+" "+"state", false, values.Struct[parameters]())
	return paramsRecord{v}
}

// paramsRecord is a wrapper around Value that sets the power to 8 if the
// parameters have not been configured.
type paramsRecord struct {
	values.Value[*parameters]
}

// Get loads the parameters, initializing them to the default values if they
// have not been set.
func (p paramsRecord) Get() (*parameters, error) {
	v, err := p.Value.Get()
	switch {
	case err == nil:
		return v, nil
	case !errors.Is(err, errors.NotFound):
		return nil, errors.UnknownError.Wrap(err)
	}

	// TODO Allow power to be configurable?
	v = new(parameters)
	v.Power = 8
	v.Mask = v.Power - 1
	err = p.Value.Put(v)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return v, nil
}

// nodeKeyAt
// We need a key to address nodes in the protocol. These nodes need a unique key
// for debugging purposes.
// We return the key with height number of bits followed by a one end bit followed by all bits clear
// Heights greater than 255 (0-254) bits are not supported.
func nodeKeyAt(height uint64, key [32]byte) (nodeKey [32]byte, ok bool) {
	if height > 254 { //                Limit is 254 because one bit marks the end of the nodeKey
		return nodeKey, false //       Return a blank nodeKey and flag it didn't work
	} //
	byteCnt := height >> 3          // The byte count is height/8 (shift left by 3)
	bitCnt := height & 7            // Mask to the mod of 8 so mask with 7 or 0b111
	copy(nodeKey[:], key[:byteCnt]) // Move the bytes into the node Key
	lastByte := key[byteCnt]        // Get the byte following these bytes into lastByte
	lastByte >>= 7 - bitCnt         // Shift right all but one bit past the key
	lastByte |= 1                   // Force that bit to 1
	lastByte <<= 7 - bitCnt         // Shift left back to the original starting point
	nodeKey[byteCnt] = lastByte     // Add the last byte to the nk.  Note always add this byte
	return nodeKey, true            // Return it as good, and that it works.
}

// parseNodeKey
// Extract the height and Key fragment from a nodeKey.  The reverse operation of GetNodeKey
// Mostly useful for debugging and testing
func parseNodeKey(nodeKey [32]byte) (height uint64, key [32]byte, ok bool) { //nolint:unused
	copy(key[:], nodeKey[:])
	byteIdx := uint64(0)                     // Calculate the trailing bytes of zero
	for i := 31; i > 0 && key[i] == 0; i-- { // Look at byte 31 back to 0
		byteIdx++
	}
	byteIdx = 31 - byteIdx // Convert to bytes not zero

	lastByte := nodeKey[byteIdx]
	if lastByte == 0 {
		return height, key, false
	}
	bit := uint64(1)
	bitMask := byte(1)
	for lastByte&bitMask == 0 {
		bit++
		bitMask <<= 1
	}
	key[byteIdx] ^= bitMask
	return byteIdx*8 + 8 - bit, key, true
}

// getRoot returns the root branch node, creating it if necessary.
func (b *BPT) getRoot() *branch {
	return values.GetOrCreate(b, &b.root, (*BPT).newRoot).branch
}

func (b *BPT) newRoot() *rootRecord {
	e := new(branch)
	e.bpt = b
	e.Height = 0
	e.Key, _ = nodeKeyAt(0, [32]byte{})
	return &rootRecord{e}
}

// Insert updates or inserts a hash for the given key. Insert may defer the
// actual update.
func (b *BPT) Insert(key, hash [32]byte) error {
	if b.pending == nil {
		b.pending = map[[32]byte][32]byte{}
	}
	b.pending[key] = hash
	return nil
}

// Get retrieves the latest hash associated with the given key.
func (b *BPT) Get(key [32]byte) ([32]byte, error) {
	if v, ok := b.pending[key]; ok {
		return v, nil
	}

	e, err := b.getRoot().getLeaf(key)
	if err != nil {
		return [32]byte{}, errors.UnknownError.Wrap(err)
	}
	return e.Hash, nil
}

// getLeaf walks the tree and returns the leaf node for the given key.
func (e *branch) getLeaf(key [32]byte) (*leaf, error) {
again:
	f, err := e.getAt(key)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	switch f := (*f).(type) {
	case *leaf:
		if f.Key == key {
			return f, nil
		}
	case *branch:
		// Recurse, but not actually
		e = f
		goto again
	}
	return nil, errors.NotFound.WithFormat("key %x not found", key)
}

// getBranch walks the tree and returns the branch node for the given key.
func (e *branch) getBranch(key [32]byte) (*branch, error) {
again:
	if e.Key == key {
		return e, nil
	}

	f, err := e.getAt(key)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	g, ok := (*f).(*branch)
	if !ok {
		return nil, errors.NotFound.WithFormat("branch %x not found", key)
	}

	// Recurse, but not actually
	e = g
	goto again
}

// Resolve implements [database.Record].
func (b *BPT) Resolve(key *database.Key) (database.Record, *database.Key, error) {
	if key.Len() == 0 {
		return nil, nil, errors.InternalError.With("bad key for bpt")
	}

	// Execute any pending updates
	err := b.executePending()
	if err != nil {
		return nil, nil, errors.UnknownError.Wrap(err)
	}

	if key.Get(0) == "Root" {
		return b.getState(), key.SliceI(1), nil
	}

	nodeKey, ok := key.Get(0).([32]byte)
	if !ok {
		return nil, nil, errors.InternalError.With("bad key for bpt")
	}
	e, err := b.getRoot().getBranch(nodeKey)
	if err != nil {
		return nil, nil, errors.UnknownError.WithFormat("bad key for BPT: %x", err)
	}

	// Ensure the node is loaded
	err = e.load()
	if err != nil {
		return nil, nil, errors.UnknownError.WithFormat("load node: %w", err)
	}

	return nodeRecord{e}, key.SliceI(1), nil
}
