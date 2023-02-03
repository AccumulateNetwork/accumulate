// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"bytes"
	"math/big"
	"sort"

	sortutil "gitlab.com/accumulatenetwork/accumulate/internal/util/sort"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func (s *TransactionStatus) Delivered() bool { return s.Code == errors.Delivered || s.Failed() }
func (s *TransactionStatus) Remote() bool    { return s.Code == errors.Remote }
func (s *TransactionStatus) Pending() bool   { return s.Code == errors.Pending }
func (s *TransactionStatus) Failed() bool    { return !s.Code.Success() }
func (s *TransactionStatus) CodeNum() uint64 { return uint64(s.Code) }

// Set sets the status code and the error.
func (s *TransactionStatus) Set(err error) {
	s.Error = errors.UnknownError.Wrap(err).(*errors.Error)
	if s.Error.Code == 0 {
		s.Code = errors.UnknownError
	} else {
		s.Code = s.Error.Code
	}
}

func (s *TransactionStatus) AddAnchorSigner(signature KeySignature) {
	ptr, new := sortutil.BinaryInsert(&s.AnchorSigners, func(key []byte) int {
		return bytes.Compare(key, signature.GetPublicKey())
	})
	if new {
		*ptr = signature.GetPublicKey()
	}
}

// AddSigner adds a signer to the object's list of signer using a binary search
// to ensure ordering.
func (s *TransactionStatus) AddSigner(signer Signer2) {
	// Find the matching entry
	ptr, new := sortutil.BinaryInsert(&s.Signers, func(entry Signer) int { return entry.GetUrl().Compare(signer.GetUrl()) })

	// Do nothing if the entry exists and the version is not newer
	if !new && signer.GetVersion() <= (*ptr).GetVersion() {
		return
	}

	// Update the entry
	*ptr = MakeLiteSigner(signer)
}

func (s *TransactionStatus) GetSigner(entry *url.URL) (Signer, bool) {
	// Find the matching entry
	i, found := sortutil.Search(s.Signers, func(e Signer) int { return e.GetUrl().Compare(entry) })

	// No match
	if !found {
		return nil, false
	}

	return s.Signers[i], true
}

func (s *TransactionStatus) FindSigners(authority *url.URL) []Signer {
	// Find the first signer for the given authority. This depends on the fact
	// that signers are always children of authorities. Or in the case of lite
	// token accounts, equal to (for now).
	i, found := sortutil.Search(s.Signers, func(e Signer) int { return e.GetUrl().Compare(authority) })

	// Past the end of the list, no match
	if i >= len(s.Signers) {
		return nil
	}

	// Entry matches (lite token account)
	if found {
		return []Signer{s.Signers[i]}
	}

	// Authority is not the parent of the entry, no match
	if !authority.ParentOf(s.Signers[i].GetUrl()) {
		return nil
	}

	// Find the first signer that is not a child of the given authority
	j := sort.Search(len(s.Signers), func(i int) bool {
		// For some I, Search expects this function to return false for
		// slice[:i] and true for slice[i:]. If the entry sorts before the
		// authority, return false. If the entry is the child of the authority,
		// return false. If the entry is not a child and sorts after, return
		// true. That will return the end of the range of entries that are
		// children of the signer.
		if s.Signers[i].GetUrl().Compare(authority) < 0 {
			return false
		}
		return !authority.ParentOf(s.Signers[i].GetUrl())
	})
	return s.Signers[i:j]
}

// IsUser returns true if the transaction type is user.
func (t TransactionType) IsUser() bool {
	return TransactionTypeUnknown < t && t.GetEnumValue() <= TransactionMaxUser.GetEnumValue()
}

// IsSynthetic returns true if the transaction type is synthetic.
func (t TransactionType) IsSynthetic() bool {
	return TransactionMaxUser.GetEnumValue() < t.GetEnumValue() && t.GetEnumValue() <= TransactionMaxSynthetic.GetEnumValue()
}

// IsSystem returns true if the transaction type is system.
func (t TransactionType) IsSystem() bool {
	return TransactionMaxSynthetic.GetEnumValue() < t.GetEnumValue() && t.GetEnumValue() <= TransactionMaxSystem.GetEnumValue()
}

// IsAnchor returns true if the transaction type is directory or block validator
// anchor.
func (t TransactionType) IsAnchor() bool {
	switch t {
	case TransactionTypeDirectoryAnchor,
		TransactionTypeBlockValidatorAnchor:
		return true
	}
	return false
}

type SyntheticTransaction interface {
	TransactionBody
}

func (tx *SendTokens) AddRecipient(to *url.URL, amount *big.Int) {
	recipient := new(TokenRecipient)
	recipient.Url = to
	recipient.Amount = *amount
	tx.To = append(tx.To, recipient)
}

// TransactionStatusError wraps a [TransactionStatus] and implements [error].
type TransactionStatusError struct {
	*TransactionStatus
}

// AsError returns nil or the status wrapped as a [TransactionStatusError].
func (s *TransactionStatus) AsError() error {
	if s.Error == nil {
		return nil
	}
	return TransactionStatusError{s}
}

func (e TransactionStatusError) Error() string {
	return e.TransactionStatus.Error.Error()
}

func (e TransactionStatusError) Unwrap() error {
	return e.TransactionStatus.Error
}

// NewErrorStatus returns a new transaction status for the given ID with the
// given error.
func NewErrorStatus(id *url.TxID, err error) *TransactionStatus {
	st := new(TransactionStatus)
	st.TxID = id
	st.Set(err)
	return st
}
