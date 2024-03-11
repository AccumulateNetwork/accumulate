// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"bytes"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/bpt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func newEventsSet[T any](c *AccountEvents, set eventsSetBase[T], getKey func(T) *record.Key, getHash func(T) []byte) *eventsSet[T] {
	return &eventsSet[T]{
		eventsSetBase: set,       // The value set
		bpt:           c.BPT(),   // The events BPT
		baseKey:       set.Key(), // The value set's key
		getKey:        getKey,    // A function that returns a key for a set element, for the BPT
		getHash:       getHash,   // A function that returns a hash for a set element, for the BPT
	}
}

func (h *AccountEventsMinor) newVotes(params accountEventsMinorVotesKey) values.Set[*protocol.AuthoritySignature] {
	e := newEventsSet(
		h.parent,
		h.baseNewVotes(params).(eventsSetBase[*protocol.AuthoritySignature]),
		getAuthSigKey,
		(*protocol.AuthoritySignature).Hash,
	)
	return &blockEventSet[*protocol.AuthoritySignature]{h, params.Block, *e}
}

func (h *AccountEventsMajor) newPending(params accountEventsMajorPendingKey) values.Set[*url.TxID] {
	e := newEventsSet(
		h.parent,
		h.baseNewPending(params).(eventsSetBase[*url.TxID]),
		getTxIDKey,
		(*url.TxID).HashSlice,
	)
	return &blockEventSet[*url.TxID]{h, params.Block, *e}
}

func (h *AccountEventsBacklog) newExpired() values.Set[*url.TxID] {
	return newEventsSet(
		h.parent,
		h.baseNewExpired().(eventsSetBase[*url.TxID]),
		getTxIDKey,
		(*url.TxID).HashSlice,
	)
}

func getAuthSigKey(a *protocol.AuthoritySignature) *record.Key {
	return record.NewKey(a.Authority, a.TxID.Hash())
}

func getTxIDKey(v *url.TxID) *record.Key {
	return record.NewKey(v.Hash())
}

// getVoteKeys indexes the Votes attribute, listing all of the blocks that have
// scheduled events.
func (c *AccountEventsMinor) getVoteKeys() ([]accountEventsMinorVotesKey, error) {
	return getBlocksAs[accountEventsMinorVotesKey](c)
}

// getPendingKeys indexes the Pending attribute, listing all of the blocks that have
// scheduled events.
func (c *AccountEventsMajor) getPendingKeys() ([]accountEventsMajorPendingKey, error) {
	return getBlocksAs[accountEventsMajorPendingKey](c)
}

// blockEventSet ensures blocks are added to the block list.
type blockEventSet[T any] struct {
	parent interface{ Blocks() values.Set[uint64] }
	block  uint64
	eventsSet[T]
}

var _ postRestorer = (*blockEventSet[any])(nil)

func (b *blockEventSet[T]) Put(v []T) error {
	err := b.eventsSet.Put(v)
	if err != nil {
		return err
	}
	if len(v) == 0 {
		return nil
	}
	// Add the block to the block list (but not if the signature list is being
	// emptied)
	err = b.parent.Blocks().Add(b.block)
	return errors.UnknownError.Wrap(err)
}

func (b *blockEventSet[T]) Add(v ...T) error {
	err := b.eventsSet.Add(v...)
	if err != nil {
		return err
	}
	// Add the block to the block list
	err = b.parent.Blocks().Add(b.block)
	return errors.UnknownError.Wrap(err)
}

// getBlocksAs fetches a list of blocks converts it to an array of T where T is
// some struct type with the field Block. This is intended to be used for index
// functions that need to return a list of blocks.
func getBlocksAs[T ~struct{ Block uint64 }](c interface{ Blocks() values.Set[uint64] }) ([]T, error) {
	b, err := c.Blocks().Get()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	k := make([]T, len(b))
	for i, b := range b {
		k[i] = T{b}
	}
	return k, nil
}

// compareHeldAuthSig compares authority signatures by authority and transaction
// hash. If a new signature is received that changes the authority's vote, this
// will overwrite the previous vote.
func compareHeldAuthSig(v, u *protocol.AuthoritySignature) int {
	c := v.Authority.Compare(u.Authority)
	if c != 0 {
		return c
	}

	// Use the hash but not the account. The account will *probably* always be
	// the same, but better safe than sorry.
	a, b := v.TxID.Hash(), u.TxID.Hash()
	return bytes.Compare(a[:], b[:])
}

type eventsSetBase[T any] interface {
	values.Set[T]
	database.Value
}

type eventsSet[T any] struct {
	eventsSetBase[T]
	bpt     *bpt.BPT
	baseKey *record.Key
	getKey  func(T) *record.Key
	getHash func(T) []byte
}

var _ postRestorer = (*eventsSet[any])(nil)
var _ database.Value = (*eventsSet[any])(nil)

func (e *eventsSet[T]) postRestore() error {
	// The the restored values
	v, err := e.Get()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Update the BPT
	for _, v := range v {
		err = e.bpt.Insert(
			e.baseKey.AppendKey(e.getKey(v)),
			*(*[32]byte)(e.getHash(v)))
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	return nil
}

func (e *eventsSet[T]) Resolve(key *record.Key) (record.Record, *record.Key, error) {
	// Overriding Resolve is necessary to ensure postRestore is called.
	if key.Len() == 0 {
		return e, nil, nil
	}
	return nil, nil, errors.InternalError.With("bad key for value")
}

func (e *eventsSet[T]) Put(v []T) error {
	// Build a lookup table of existing values
	existing, err := e.Get()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	m := map[[32]byte]*record.Key{}
	for _, v := range existing {
		k := e.getKey(v)
		m[k.Hash()] = k
	}

	// Update the BPT
	for _, v := range v {
		k := e.getKey(v)
		kh := k.Hash()
		err = e.bpt.Insert(
			e.baseKey.AppendKey(k),
			*(*[32]byte)(e.getHash(v)))
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
		delete(m, kh)
	}

	// Remove any entries that no longer exist
	for _, k := range m {
		err = e.bpt.Delete(e.baseKey.AppendKey(k))
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	return e.eventsSetBase.Put(v)
}

func (e *eventsSet[T]) Add(v ...T) error {
	// Update the BPT
	for _, v := range v {
		err := e.bpt.Insert(
			e.baseKey.AppendKey(e.getKey(v)),
			*(*[32]byte)(e.getHash(v)))
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	return e.eventsSetBase.Add(v...)
}

func (e *eventsSet[T]) Remove(v T) error {
	// Update the BPT
	err := e.bpt.Delete(e.baseKey.AppendKey(e.getKey(v)))
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	return e.eventsSetBase.Remove(v)
}
