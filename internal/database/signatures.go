// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"sort"

	sortutil "gitlab.com/accumulatenetwork/accumulate/internal/util/sort"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func compareSignatureSetEntries(a, b *SignatureSetEntry) int {
	return int(a.KeyIndex) - int(b.KeyIndex)
}

func compareVoteEntries(a, b *VoteEntry) int {
	return a.Authority.Compare(b.Authority)
}

// RecordHistory adds the message to the signature chain and history.
func (c *AccountTransaction) RecordHistory(msg messaging.Message) error {
	// The count now will be the index of the new entry
	head, err := c.parent.SignatureChain().Head().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load chain head: %w", err)
	}
	index := uint64(head.Count)

	// Add the chain entry
	h := msg.Hash()
	err = c.parent.SignatureChain().Inner().AddHash(h[:], false)
	if err != nil {
		return errors.UnknownError.WithFormat("add to chain: %w", err)
	}

	// Add the history index
	err = c.History().Add(index)
	if err != nil {
		return errors.UnknownError.WithFormat("store history: %w", err)
	}

	// Add the signer to the transaction's signer list
	signerUrl := c.parent.Url()
	hash := c.key[3].([32]byte)
	err = c.parent.parent.Message(hash).Signers().Add(signerUrl)
	return errors.UnknownError.Wrap(err)
}

// FindSigners return signers that are equal to or a child of the given URL.
// FindSigners only returns an error if the signer list cannot be retrieved.
// FindSigners returns nil, if no matching signers are found.
func (m *Message) FindSigners(u *url.URL) ([]*url.URL, error) {
	signers, err := m.Signers().Get()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	i, found := sortutil.Search(signers, func(v *url.URL) int { return v.Compare(u) })
	switch {
	case found:
		// Single match
		return signers[i : i+1], nil

	case i >= len(signers):
		// No match

	case !u.ParentOf(signers[i]):
		// Entry belongs to some other authority (is this possible?)
		return nil, nil
	}

	// Find the first signer that is not a child of the given authority
	//
	// This could be more efficient - we don't need to search the entire array -
	// but these algorithms make my brain hurt and I don't want to break it
	j := sort.Search(len(signers), func(i int) bool {
		// For some I, Search expects this function to return false for
		// slice[:i] and true for slice[i:]. If the entry sorts before the
		// authority, return false. If the entry is the child of the authority,
		// return false. If the entry is not a child and sorts after, return
		// true. That will return the end of the range of entries that are
		// children of the signer.
		if signers[i].Compare(u) < 0 {
			return false
		}
		return !u.ParentOf(signers[i])
	})
	return signers[i:j], nil
}
