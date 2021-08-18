package state

import (
	"github.com/AccumulateNetwork/accumulated/types"
)

//Header information for the state object.  Each state object will contain a header
//that will consist of the chain type enumerator
type Header struct {
	Type         types.String `json:"type"`
	AdiChainPath types.String `json:"adi-chain-path"`
}

//GetHeaderSize will return the marshalled binary size of the header.
func (h *Header) GetHeaderSize() int {
	return h.Type.Size(nil) + h.AdiChainPath.Size(nil)
}

//GetType will return the chain type
func (h *Header) GetType() string {
	return string(h.Type)
}

//GetAdiChainPath returns the url to the chain of this object
func (h *Header) GetAdiChainPath() string {
	return string(h.AdiChainPath)
}

//MarshalBinary serializes the header
func (h *Header) MarshalBinary() ([]byte, error) {
	data := make([]byte, h.GetHeaderSize())
	htdata, err := h.Type.MarshalBinary()
	if err != nil {
		return nil, err
	}
	i := copy(data, htdata)

	htdata, err = h.AdiChainPath.MarshalBinary()
	if err != nil {
		return nil, err
	}

	i += copy(data[i:], htdata)
	return data, nil
}

//UnmarshalBinary deserializes the data array into the header object
func (h *Header) UnmarshalBinary(data []byte) error {

	err := h.Type.UnmarshalBinary(data)
	if err != nil {
		return err
	}

	err = h.AdiChainPath.UnmarshalBinary(data[h.Type.Size(nil):])
	if err != nil {
		return err
	}

	return nil
}
