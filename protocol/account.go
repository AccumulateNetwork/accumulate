package protocol

import (
	"encoding"
	"fmt"
)

type Account interface {
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
	Header() *AccountHeader
}

func NewChain(typ AccountType) (Account, error) {
	switch typ {
	case AccountTypeAnchor:
		return new(Anchor), nil
	case AccountTypeIdentity:
		return new(ADI), nil
	case AccountTypeTokenIssuer:
		return new(TokenIssuer), nil
	case AccountTypeTokenAccount:
		return new(TokenAccount), nil
	case AccountTypeLiteTokenAccount:
		return new(LiteTokenAccount), nil
	case AccountTypeTransaction:
		return new(TransactionState), nil
	case AccountTypePendingTransaction:
		return new(PendingTransactionState), nil
	case AccountTypeKeyPage:
		return new(KeyPage), nil
	case AccountTypeKeyBook:
		return new(KeyBook), nil
	case AccountTypeDataAccount:
		return new(DataAccount), nil
	case AccountTypeLiteDataAccount:
		return new(LiteDataAccount), nil
	case AccountTypeInternalLedger:
		return new(InternalLedger), nil
	default:
		return nil, fmt.Errorf("unknown account type %v", typ)
	}
}

func UnmarshalChain(data []byte) (Account, error) {
	var typ AccountType
	err := typ.UnmarshalBinary(data)
	if err != nil {
		return nil, err
	}

	chain, err := NewChain(typ)
	if err != nil {
		return nil, err
	}

	err = chain.UnmarshalBinary(data)
	if err != nil {
		return nil, err
	}

	return chain, nil
}
