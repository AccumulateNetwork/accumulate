package query

import (
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
)

type RequestByChainId struct {
	ChainId types.Bytes32
}

type ResponseByChainId struct {
	Data types.Bytes
}

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
	r.ChainId.FromBytes(data)

	return nil
}

func (r *ResponseByChainId) MarshalBinary() ([]byte, error) {
	return r.Data.MarshalBinary()
}

func (r *ResponseByChainId) UnmarshalBinary(data []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("error unmarshaling ResponseByChainId data %v", r)
		}
	}()
	return r.Data.UnmarshalBinary(data)
}
