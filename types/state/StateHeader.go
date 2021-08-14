package state

import (
	"github.com/AccumulateNetwork/accumulated/types"
)

type StateHeader struct {
	Type         types.String `json:"type"`
	AdiChainPath types.String `json:"adi-chain-path"`
}

func (is *StateHeader) GetHeaderSize() int {
	return is.Type.Size(nil) + is.AdiChainPath.Size(nil)
}

func (is *StateHeader) GetType() string {
	return string(is.Type)
}

func (is *StateHeader) GetAdiChainPath() string {
	return string(is.AdiChainPath)
}

func (h *StateHeader) MarshalBinary() ([]byte, error) {
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

func (h *StateHeader) UnmarshalBinary(data []byte) error {

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
