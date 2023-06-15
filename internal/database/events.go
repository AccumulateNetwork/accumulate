// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"bytes"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (h *AccountEventsMinor) Votes(block uint64) values.Set[*protocol.AuthoritySignature] {
	return &blockEventSet[*protocol.AuthoritySignature]{h, block, h.getVotes(block)}
}

func (h *AccountEventsMajor) Pending(block uint64) values.Set[*url.TxID] {
	return &blockEventSet[*url.TxID]{h, block, h.getPending(block)}
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
	values.Set[T]
}

func (b *blockEventSet[T]) Put(v []T) error {
	err := b.Set.Put(v)
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
	err := b.Set.Add(v...)
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
