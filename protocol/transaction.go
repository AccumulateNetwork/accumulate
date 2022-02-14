package protocol

import (
	"encoding"
	"encoding/json"
	"fmt"
	"math/big"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

func NewTransaction(typ TransactionType) (TransactionPayload, error) {
	switch typ {
	case TransactionTypeCreateIdentity:
		return new(CreateIdentity), nil
	case TransactionTypeCreateTokenAccount:
		return new(CreateTokenAccount), nil
	case TransactionTypeSendTokens:
		return new(SendTokens), nil
	case TransactionTypeCreateDataAccount:
		return new(CreateDataAccount), nil
	case TransactionTypeWriteData:
		return new(WriteData), nil
	case TransactionTypeWriteDataTo:
		return new(WriteDataTo), nil
	case TransactionTypeAcmeFaucet:
		return new(AcmeFaucet), nil
	case TransactionTypeCreateToken:
		return new(CreateToken), nil
	case TransactionTypeIssueTokens:
		return new(IssueTokens), nil
	case TransactionTypeBurnTokens:
		return new(BurnTokens), nil
	case TransactionTypeCreateKeyPage:
		return new(CreateKeyPage), nil
	case TransactionTypeCreateKeyBook:
		return new(CreateKeyBook), nil
	case TransactionTypeAddCredits:
		return new(AddCredits), nil
	case TransactionTypeUpdateKeyPage:
		return new(UpdateKeyPage), nil
	case TransactionTypeSignPending:
		return new(SignPending), nil
	case TransactionTypeUpdateManager:
		return new(UpdateManager), nil
	case TransactionTypeRemoveManager:
		return new(RemoveManager), nil

	case TransactionTypeSyntheticCreateChain:
		return new(SyntheticCreateChain), nil
	case TransactionTypeSyntheticWriteData:
		return new(SyntheticWriteData), nil
	case TransactionTypeSyntheticDepositTokens:
		return new(SyntheticDepositTokens), nil
	case TransactionTypeSyntheticAnchor:
		return new(SyntheticAnchor), nil
	case TransactionTypeSyntheticDepositCredits:
		return new(SyntheticDepositCredits), nil
	case TransactionTypeSyntheticBurnTokens:
		return new(SyntheticBurnTokens), nil
	case TransactionTypeSyntheticMirror:
		return new(SyntheticMirror), nil
	case TransactionTypeSegWitDataEntry:
		return new(SegWitDataEntry), nil

	case TransactionTypeInternalGenesis:
		return new(InternalGenesis), nil
	case TransactionTypeInternalTransactionsSigned:
		return new(InternalTransactionsSigned), nil
	case TransactionTypeInternalTransactionsSent:
		return new(InternalTransactionsSent), nil

	default:
		return nil, fmt.Errorf("unknown transaction type %v", typ)
	}
}

func UnmarshalTransaction(data []byte) (TransactionPayload, error) {
	var typ TransactionType
	err := typ.UnmarshalBinary(data)
	if err != nil {
		return nil, err
	}

	tx, err := NewTransaction(typ)
	if err != nil {
		return nil, err
	}

	err = tx.UnmarshalBinary(data)
	if err != nil {
		return nil, err
	}

	return tx, nil
}

func UnmarshalTransactionJSON(data []byte) (TransactionPayload, error) {
	var typ struct{ Type TransactionType }
	err := json.Unmarshal(data, &typ)
	if err != nil {
		return nil, err
	}

	tx, err := NewTransaction(typ.Type)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, tx)
	if err != nil {
		return nil, err
	}

	return tx, nil
}

type TransactionPayload interface {
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
	GetType() TransactionType
	BinarySize() int
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
	recipient.Url = to.String()
	recipient.Amount = *amount
	tx.To = append(tx.To, recipient)
}
