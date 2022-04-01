package query

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/types"
)

type RequestByChainId struct {
	ChainId types.Bytes32
}

func (*RequestByChainId) Type() types.QueryType { return types.QueryTypeChainId }

func (r *RequestByChainId) MarshalBinary() ([]byte, error) {
	return r.ChainId[:], nil
}

func (r *RequestByChainId) UnmarshalBinary(data []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("error unmarshaling RequestByChainId data %v", r)
		}
	}()

	if len(data) < 32 {
		return fmt.Errorf("insufficient data for chain id")
	}

	return r.ChainId.FromBytes(data)
}
