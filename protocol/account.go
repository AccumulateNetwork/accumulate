package protocol

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

func NewChain(typ types.ChainType) (state.Chain, error) {
	switch typ {
	case types.ChainTypeAnchor:
		return new(Anchor), nil
	case types.ChainTypeIdentity:
		return new(ADI), nil
	case types.ChainTypeTokenIssuer:
		return new(TokenIssuer), nil
	case types.ChainTypeTokenAccount:
		return new(TokenAccount), nil
	case types.ChainTypeLiteTokenAccount:
		return new(LiteTokenAccount), nil
	case types.ChainTypeTransaction:
		return new(state.Transaction), nil
	case types.ChainTypePendingTransaction:
		return new(state.PendingTransaction), nil
	case types.ChainTypeKeyPage:
		return new(KeyPage), nil
	case types.ChainTypeKeyBook:
		return new(KeyBook), nil
	case types.ChainTypeDataAccount:
		return new(DataAccount), nil
	case types.ChainTypeLiteDataAccount:
		return new(LiteDataAccount), nil
	case types.ChainTypeSyntheticTransactions:
		return new(SyntheticTransactionChain), nil
	case types.ChainTypeInternalLedger:
		return new(InternalLedger), nil
	default:
		return nil, fmt.Errorf("unknown chain type %v", typ)
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
