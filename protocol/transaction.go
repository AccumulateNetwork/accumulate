package protocol

import (
	"errors"
	"math/big"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/internal/sortutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

// SetHash sets the hash returned by GetHash. This will return an error if the
// body type is not Remote.
func (t *Transaction) SetHash(hash []byte) error {
	if t.Body.Type() != TransactionTypeRemote {
		return errors.New("cannot set the hash: not a remote transaction")
	}
	t.hash = hash
	return nil
}

// AddSigner adds a signer to the object's list of signer using a binary search
// to ensure ordering.
func (s *TransactionStatus) AddSigner(signer Signer) {
	// Initial signer
	if len(s.Signers) == 0 {
		s.Initiator = signer.GetUrl()
	}

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

// IsSystem returns true if the transaction type is internal.
func (t TransactionType) IsSystem() bool {
	return TransactionMaxSynthetic.GetEnumValue() < t.GetEnumValue() && t.GetEnumValue() <= TransactionMaxSystem.GetEnumValue()
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
