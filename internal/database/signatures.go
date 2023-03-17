// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func compareSignatureSetEntries(a, b *SignatureSetEntry) int {
	return int(a.KeyIndex) - int(b.KeyIndex)
}

func compareVoteEntries(a, b *VoteEntry) int {
	return a.Authority.Compare(b.Authority)
}

func (c *AccountTransactionSignatures) Active() record.Set[*SignatureSetEntry] {
	return &accountTransactionSignaturesActive{c.getActive(), c}
}

func (a *AccountTransaction) hash() [32]byte {
	return a.key[1+len(a.parent.key)].([32]byte)
}

// accountTransactionSignaturesActive is a wrapper for the active signature [record.Set] that records
// the signer in Message(hash).Signers.
type accountTransactionSignaturesActive struct {
	record.Set[*SignatureSetEntry]
	parent *AccountTransactionSignatures
}

func (a *accountTransactionSignaturesActive) updateIndices(v []*SignatureSetEntry) error {
	if len(v) == 0 {
		return nil
	}

	// Update history
	var u []uint64
	for _, v := range v {
		u = append(u, v.ChainIndex)
	}
	err := a.parent.History().Add(u...)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Record the signerUrl URL in the message's signerUrl list
	signerUrl := a.parent.parent.parent.Url()
	hash := a.parent.parent.hash()
	err = a.parent.parent.parent.parent.Message(hash).Signers().Add(signerUrl)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	var signer protocol.Signer
	err = a.parent.parent.parent.Main().GetAs(&signer)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Record the signer URL in the transaction status for backwards
	// compatibility; TODO remove
	err = a.parent.parent.parent.parent.Transaction2(hash).ensureSigner(signer)
	return errors.UnknownError.Wrap(err)
}

func (a *accountTransactionSignaturesActive) Add(v ...*SignatureSetEntry) error {
	err := a.Set.Add(v...)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	err = a.updateIndices(v)
	return errors.UnknownError.Wrap(err)
}

func (a *accountTransactionSignaturesActive) Put(v []*SignatureSetEntry) error {
	err := a.Set.Put(v)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	err = a.updateIndices(v)
	return errors.UnknownError.Wrap(err)
}
