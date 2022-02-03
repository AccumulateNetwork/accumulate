package protocol

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/state"
)

func NewChain(typ types.AccountType) (state.Chain, error) {
	switch typ {
	case types.AccountTypeAnchor:
		return new(Anchor), nil
	case types.AccountTypeIdentity:
		return new(ADI), nil
	case types.AccountTypeTokenIssuer:
		return new(TokenIssuer), nil
	case types.AccountTypeTokenAccount:
		return new(TokenAccount), nil
	case types.AccountTypeLiteTokenAccount:
		return new(LiteTokenAccount), nil
	case types.AccountTypeTransaction:
		return new(state.Transaction), nil
	case types.AccountTypePendingTransaction:
		return new(state.PendingTransaction), nil
	case types.AccountTypeKeyPage:
		return new(KeyPage), nil
	case types.AccountTypeKeyBook:
		return new(KeyBook), nil
	case types.AccountTypeDataAccount:
		return new(DataAccount), nil
	case types.AccountTypeLiteDataAccount:
		return new(LiteDataAccount), nil
	case types.AccountTypeInternalLedger:
		return new(InternalLedger), nil
	default:
		return nil, fmt.Errorf("unknown account type %v", typ)
	}
}

func UnmarshalChain(data []byte) (state.Chain, error) {
	typ, err := state.ChainType(data)
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
