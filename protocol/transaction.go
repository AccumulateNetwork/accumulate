package protocol

import (
	"math/big"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

// Deprecated: use TransactionBody
type TransactionPayload = TransactionBody

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
	return TransactionTypeUnknown < t && t.ID() <= TransactionMaxUser.ID()
}

// IsSynthetic returns true if the transaction type is synthetic.
func (t TransactionType) IsSynthetic() bool {
	return TransactionMaxUser.ID() < t.ID() && t.ID() <= TransactionMaxSynthetic.ID()
}

// IsInternal returns true if the transaction type is internal.
func (t TransactionType) IsInternal() bool {
	return TransactionMaxSynthetic.ID() < t.ID() && t.ID() <= TransactionMaxInternal.ID()
}

type SyntheticTransaction interface {
	TransactionPayload
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
