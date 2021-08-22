package state

import (
	"bytes"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
)

//Header information for the state object.  Each state object will contain a header
//that will consist of the chain type enumerator
type Header struct {
	Type         types.Bytes32 `json:"type"`
	AdiChainPath types.String  `json:"adi-chain-path"`
}

//GetHeaderSize will return the marshalled binary size of the header.
func (h *Header) GetHeaderSize() int {
	return 32 + h.AdiChainPath.Size(nil)
}

//GetType will return the chain type
func (h *Header) GetType() *types.Bytes32 {
	return &h.Type
}

//GetAdiChainPath returns the url to the chain of this object
func (h *Header) GetAdiChainPath() string {
	return string(h.AdiChainPath)
}

//MarshalBinary serializes the header
func (h *Header) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	buffer.Write(h.Type.Bytes())

	htdata, err := h.AdiChainPath.MarshalBinary()
	if err != nil {
		return nil, err
	}

	buffer.Write(htdata)

	return buffer.Bytes(), nil
}

//UnmarshalBinary deserializes the data array into the header object
func (h *Header) UnmarshalBinary(data []byte) error {

	if len(data) < 32 {
		return fmt.Errorf("state header buffer too short for unmarshal")
	}
	i := copy(h.Type[:], data[:32])

	err := h.AdiChainPath.UnmarshalBinary(data[i:])
	if err != nil {
		return err
	}

	return nil
}
