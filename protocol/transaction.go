package protocol

import (
	"math/big"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

// AddSigner adds a signer to the object's list of signer using a binary search
// to ensure ordering.
func (s *TransactionStatus) AddSigner(signer Signer) {
	signer = MakeLiteSigner(signer)

	// Initial signer
	if len(s.Signers) == 0 {
		s.Initiator = signer.GetUrl()
		s.Signers = []Signer{signer}
		return
	}

	// Find the matching entry
	i := sort.Search(len(s.Signers), func(i int) bool {
		return s.Signers[i].GetUrl().Compare(signer.GetUrl()) >= 0
	})

	// Append to the list
	if i >= len(s.Signers) {
		s.Signers = append(s.Signers, signer)
		return
	}

	// Update the existing entry
	if s.Signers[i].GetUrl().Equal(signer.GetUrl()) {
		if signer.GetVersion() > s.Signers[i].GetVersion() {
			s.Signers[i] = signer
		}
		return
	}

	// Insert within the list
	s.Signers = append(s.Signers, nil)
	copy(s.Signers[i+1:], s.Signers[i:])
	s.Signers[i] = signer
}

func (s *TransactionStatus) GetSigner(entry *url.URL) (Signer, bool) {
	// Find the matching entry
	i := sort.Search(len(s.Signers), func(i int) bool {
		return s.Signers[i].GetUrl().Compare(entry) >= 0
	})

	// No match
	if i > len(s.Signers) || !s.Signers[i].GetUrl().Equal(entry) {
		return nil, false
	}

	return s.Signers[i], true
}

func (s *TransactionStatus) FindSigners(authority *url.URL) []Signer {
	// Find the first signer for the given authority. This depends on the fact
	// that signers are always children of authorities. Or in the case of lite
	// token accounts, equal to (for now).
	i := sort.Search(len(s.Signers), func(i int) bool {
		return s.Signers[i].GetUrl().Compare(authority) >= 0
	})

	// Past the end of the list, no match
	if i >= len(s.Signers) {
		return nil
	}

	// Entry matches (lite token account)
	if s.Signers[i].GetUrl().Equal(authority) {
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
