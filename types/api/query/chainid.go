package query

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
)

type RequestByChainId struct {
	ChainId types.Bytes32
}

type ResponseByChainId struct {
	protocol.Object
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

func (r *ResponseByChainId) MarshalBinary() ([]byte, error) {
	return r.Object.MarshalBinary()
}

func (r *ResponseByChainId) UnmarshalBinary(data []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("error unmarshaling ResponseByChainId data %v", r)
		}
	}()
	return r.Object.UnmarshalBinary(data)
}
