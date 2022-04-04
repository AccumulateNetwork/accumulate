package protocol

import (
	"errors"
	"math/big"
	"sort"

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
func (s *TransactionStatus) AddSigner(signer *url.URL) {
	// Initial signer
	if len(s.Signers) == 0 {
		s.Initiator = signer
		s.Signers = []*url.URL{signer}
		return
	}

	// Find the matching entry
	i := sort.Search(len(s.Signers), func(i int) bool {
		return s.Signers[i].Compare(signer) >= 0
	})

	// Append to the list
	if i >= len(s.Signers) {
		s.Signers = append(s.Signers, signer)
		return
	}

	// A matching entry exists
	if signer.Equal(s.Signers[i]) {
		return
	}

	// Insert within the list
	s.Signers = append(s.Signers, nil)
	copy(s.Signers[i+1:], s.Signers[i:])
	s.Signers[i] = signer
}

func (s *TransactionStatus) FindSigners(authority *url.URL) []*url.URL {
	// Find the first signer for the given authority. This depends on the fact
	// that signers are always children of authorities. Or in the case of lite
	// token accounts, equal to (for now).
	i := sort.Search(len(s.Signers), func(i int) bool {
		return s.Signers[i].Compare(authority) >= 0
	})

	// Past the end of the list, no match
	if i >= len(s.Signers) {
		return nil
	}

	// Entry matches (lite token account)
	if s.Signers[i].Equal(authority) {
		return []*url.URL{authority}
	}

	// Authority is not the parent of the entry, no match
	if !authority.ParentOf(s.Signers[i]) {
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
		if s.Signers[i].Compare(authority) < 0 {
			return false
		}
		return !authority.ParentOf(s.Signers[i])
	})
	return s.Signers[i:j]
}

func NewTransaction(typ TransactionType) (TransactionBody, error) {
	return NewTransactionBody(typ)
}

func UnmarshalTransaction(data []byte) (TransactionBody, error) {
	return UnmarshalTransactionBody(data)
}

func UnmarshalTransactionJSON(data []byte) (TransactionBody, error) {
	return UnmarshalTransactionBodyJSON(data)
}

// IsUser returns true if the transaction type is user.
func (t TransactionType) IsUser() bool {
	return TransactionTypeUnknown < t && t.GetEnumValue() <= TransactionMaxUser.GetEnumValue()
}

// IsSynthetic returns true if the transaction type is synthetic.
func (t TransactionType) IsSynthetic() bool {
	return TransactionMaxUser.GetEnumValue() < t.GetEnumValue() && t.GetEnumValue() <= TransactionMaxSynthetic.GetEnumValue()
}

// IsInternal returns true if the transaction type is internal.
func (t TransactionType) IsInternal() bool {
	return TransactionMaxSynthetic.GetEnumValue() < t.GetEnumValue() && t.GetEnumValue() <= TransactionMaxInternal.GetEnumValue()
}

type SyntheticTransaction interface {
	TransactionBody
	GetCause() [32]byte
}

func (tx *SyntheticCreateChain) GetCause() [32]byte    { return tx.Cause }
func (tx *SyntheticWriteData) GetCause() [32]byte      { return tx.Cause }
func (tx *SyntheticDepositTokens) GetCause() [32]byte  { return tx.Cause }
func (tx *SyntheticDepositCredits) GetCause() [32]byte { return tx.Cause }
func (tx *SyntheticBurnTokens) GetCause() [32]byte     { return tx.Cause }
func (tx *SegWitDataEntry) GetCause() [32]byte         { return tx.Cause }

func (tx *SyntheticCreateChain) Create(chains ...Account) error {
	for _, chain := range chains {
		b, err := chain.MarshalBinary()
		if err != nil {
			return err
		}

		tx.Chains = append(tx.Chains, ChainParams{Data: b})
	}
	return nil
}

func (tx *SyntheticCreateChain) Update(chains ...Account) error {
	for _, chain := range chains {
		b, err := chain.MarshalBinary()
		if err != nil {
			return err
		}

		tx.Chains = append(tx.Chains, ChainParams{Data: b, IsUpdate: true})
	}
	return nil
}

func (tx *SendTokens) AddRecipient(to *url.URL, amount *big.Int) {
	recipient := new(TokenRecipient)
	recipient.Url = to
	recipient.Amount = *amount
	tx.To = append(tx.To, recipient)
}
