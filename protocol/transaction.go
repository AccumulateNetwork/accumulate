package protocol

import (
	"encoding"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

func NewTransaction(typ types.TransactionType) (TransactionPayload, error) {
	switch typ {
	case types.TxTypeCreateIdentity:
		return new(CreateIdentity), nil
	case types.TxTypeCreateTokenAccount:
		return new(CreateTokenAccount), nil
	case types.TxTypeSendTokens:
		return new(SendTokens), nil
	case types.TxTypeCreateDataAccount:
		return new(CreateDataAccount), nil
	case types.TxTypeWriteData:
		return new(WriteData), nil
	case types.TxTypeWriteDataTo:
		return new(WriteDataTo), nil
	case types.TxTypeAcmeFaucet:
		return new(AcmeFaucet), nil
	case types.TxTypeCreateToken:
		return new(CreateToken), nil
	case types.TxTypeIssueTokens:
		return new(IssueTokens), nil
	case types.TxTypeBurnTokens:
		return new(BurnTokens), nil
	case types.TxTypeCreateKeyPage:
		return new(CreateKeyPage), nil
	case types.TxTypeCreateKeyBook:
		return new(CreateKeyBook), nil
	case types.TxTypeAddCredits:
		return new(AddCredits), nil
	case types.TxTypeUpdateKeyPage:
		return new(UpdateKeyPage), nil
	case types.TxTypeSignPending:
		return new(SignPending), nil

	case types.TxTypeSyntheticCreateChain:
		return new(SyntheticCreateChain), nil
	case types.TxTypeSyntheticWriteData:
		return new(SyntheticWriteData), nil
	case types.TxTypeSyntheticDepositTokens:
		return new(SyntheticDepositTokens), nil
	case types.TxTypeSyntheticAnchor:
		return new(SyntheticAnchor), nil
	case types.TxTypeSyntheticDepositCredits:
		return new(SyntheticDepositCredits), nil
	case types.TxTypeSyntheticBurnTokens:
		return new(SyntheticBurnTokens), nil
	case types.TxTypeSyntheticMirror:
		return new(SyntheticMirror), nil
	case types.TxTypeSegWitDataEntry:
		return new(SegWitDataEntry), nil

	case types.TxTypeInternalGenesis:
		return new(InternalGenesis), nil
	case types.TxTypeInternalTransactionsSigned:
		return new(InternalTransactionsSigned), nil
	case types.TxTypeInternalTransactionsSent:
		return new(InternalTransactionsSent), nil

	default:
		return nil, fmt.Errorf("unknown transaction type %v", typ)
	}
}

func UnmarshalTransaction(data []byte) (TransactionPayload, error) {
	var typ types.TransactionType
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
	var typ struct{ Type types.TransactionType }
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
	GetType() types.TxType
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

func (tx *SyntheticCreateChain) Create(chains ...state.Chain) error {
	for _, chain := range chains {
		b, err := chain.MarshalBinary()
		if err != nil {
			return err
		}

		tx.Chains = append(tx.Chains, ChainParams{Data: b})
	}
	return nil
}

func (tx *SyntheticCreateChain) Update(chains ...state.Chain) error {
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
