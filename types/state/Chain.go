package state

import (
	"bytes"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/types"
)

//Chain information for the state object.  Each state object will contain a header
//that will consist of the chain type enumerator
type Chain struct {
	Entry
	ChainUrl types.String  `json:"url" form:"url" query:"url" validate:"required,alphanum"`
	Type     types.Bytes32 `json:"type" form:"type" query:"type" validate:"required"`
}

func NewChain(chainUrl types.String, chainType types.Bytes) *Chain {
	chain := &Chain{}
	chain.SetHeader(chainUrl, chainType)
	return chain
}

//SetHeader sets the data for a chain header
func (h *Chain) SetHeader(chainUrl types.String, chainType types.Bytes) {
	h.ChainUrl = chainUrl
	copy(h.Type[:], chainType)
}

//GetHeaderSize will return the marshalled binary size of the header.
func (h *Chain) GetHeaderSize() int {
	return 32 + h.ChainUrl.Size(nil)
}

//GetType will return the chain type
func (h *Chain) GetType() *types.Bytes32 {
	return &h.Type
}

//GetAdiChainPath returns the url to the chain of this object
func (h *Chain) GetChainUrl() string {
	return *h.ChainUrl.AsString()
}

//MarshalBinary serializes the header
func (h *Chain) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	buffer.Write(h.Type.Bytes())

	htdata, err := h.ChainUrl.MarshalBinary()
	if err != nil {
		return nil, err
	}

	buffer.Write(htdata)

	return buffer.Bytes(), nil
}

//UnmarshalBinary deserializes the data array into the header object
func (h *Chain) UnmarshalBinary(data []byte) error {

	if len(data) < 32 {
		return fmt.Errorf("state header buffer too short for unmarshal")
	}
	i := copy(h.Type[:], data[:32])

	err := h.ChainUrl.UnmarshalBinary(data[i:])
	if err != nil {
		return err
	}

	return nil
}
