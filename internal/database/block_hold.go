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
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// minorBlockEntries ensures blocks are added to the block list.
type minorBlockEntries struct {
	parent *AccountEventMinor
	block  uint64
	values.Set[*protocol.AuthoritySignature]
}

func (h *AccountEventMinor) Votes(block uint64) values.Set[*protocol.AuthoritySignature] {
	return &minorBlockEntries{h, block, h.getVotes(block)}
}

func (e *minorBlockEntries) Put(v []*protocol.AuthoritySignature) error {
	err := e.Set.Put(v)
	if err != nil {
		return err
	}
	if len(v) == 0 {
		return nil
	}
	// Add the block to the block list (but not if the signature list is being
	// emptied)
	err = e.parent.Blocks().Add(e.block)
	return errors.UnknownError.Wrap(err)
}

func (e *minorBlockEntries) Add(v ...*protocol.AuthoritySignature) error {
	err := e.Set.Add(v...)
	if err != nil {
		return err
	}
	// Add the block to the block list
	err = e.parent.Blocks().Add(e.block)
	return errors.UnknownError.Wrap(err)
}

// getVoteKeys indexes the Votes attribute, listing all of the blocks that have
// scheduled events.
func (h *AccountEventMinor) getVoteKeys() ([]accountEventMinorVotesKey, error) {
	b, err := h.Blocks().Get()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	k := make([]accountEventMinorVotesKey, len(b))
	for i, b := range b {
		k[i] = accountEventMinorVotesKey{b}
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
