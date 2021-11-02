package protocol

import (
	"encoding"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

type TransactionPayload interface {
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
	GetType() types.TxType
}

type SyntheticTransaction interface {
	TransactionPayload
	GetCause() [32]byte
}

func (scc *SyntheticCreateChain) GetCause() [32]byte {
	return scc.Cause
}

func (scc *SyntheticCreateChain) Create(chains ...state.Chain) error {
	for _, chain := range chains {
		b, err := chain.MarshalBinary()
		if err != nil {
			return err
		}

		scc.Chains = append(scc.Chains, ChainParams{Data: b})
	}
	return nil
}

func (scc *SyntheticCreateChain) Update(chains ...state.Chain) error {
	for _, chain := range chains {
		b, err := chain.MarshalBinary()
		if err != nil {
			return err
		}

		scc.Chains = append(scc.Chains, ChainParams{Data: b, IsUpdate: true})
	}
	return nil
}
