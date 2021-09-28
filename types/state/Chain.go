package state

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/smt/common"
	"github.com/AccumulateNetwork/accumulated/types"
)

//Chain information for the state object.  Each state object will contain a header
//that will consist of the chain type enumerator
type Chain struct {
	Entry
	Type      uint64        `json:"type" form:"type" query:"type" validate:"required"`
	SigSpecId types.Bytes32 `json:"sigSpecId"` //this is the chain id for the sig spec for the chain
	ChainUrl  types.String  `json:"url" form:"url" query:"url" validate:"required,alphanum"`
}

func NewChain(chainUrl types.String, chainType uint64) *Chain {
	chain := &Chain{}
	chain.SetHeader(chainUrl, chainType)
	return chain
}

//SetHeader sets the data for a chain header
func (h *Chain) SetHeader(chainUrl types.String, chainType uint64) {
	h.ChainUrl = chainUrl
	h.Type = chainType
}

//GetHeaderSize will return the marshalled binary size of the header.
func (h *Chain) GetHeaderSize() int {
	var buf [8]byte
	i := binary.PutUvarint(buf[:], h.Type)
	return i + h.ChainUrl.Size(nil)
}

//GetType will return the chain type
func (h *Chain) GetType() uint64 {
	return h.Type
}

//GetAdiChainPath returns the url to the chain of this object
func (h *Chain) GetChainUrl() string {
	return *h.ChainUrl.AsString()
}

//MarshalBinary serializes the header
func (h *Chain) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	buffer.Write(common.Uint64Bytes(h.Type))

	urlData, err := h.ChainUrl.MarshalBinary()
	if err != nil {
		return nil, err
	}

	buffer.Write(urlData)

	return buffer.Bytes(), nil
}

//UnmarshalBinary deserializes the data array into the header object
func (h *Chain) UnmarshalBinary(data []byte) error {
	if len(data[:]) < 8 {
		return fmt.Errorf("state header buffer too short for unmarshal")
	}
	h.Type, data = common.BytesUint64(data)

	err := h.ChainUrl.UnmarshalBinary(data)
	if err != nil {
		return err
	}

	return nil
}

func UnmarshalChain(data []byte) (*Chain, error) {
	ch := new(Chain)
	err := ch.UnmarshalBinary(data)
	if err != nil {
		return nil, err
	}
	return ch, nil
}
