package synthetic

import (
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
)

type Header struct {
	Txid           types.Bytes32 `json:"txid"`
	SourceAdiChain types.Bytes32 `json:"sourceAdiChain"`
	SourceChainId  types.Bytes32 `json:"sourceChainId"`
}

const HeaderLen = 32 * 3

func (h *Header) MarshalBinary() ([]byte, error) {
	data := make([]byte, HeaderLen)
	i := copy(data[:], h.Txid[:])
	i += copy(data[i:], h.SourceAdiChain[:])
	i += copy(data[i:], h.SourceChainId[:])
	return data, nil
}

func (h *Header) UnmarshalBinary(data []byte) error {
	if HeaderLen > len(data) {
		return fmt.Errorf("insufficient data to unmarshal synthetic transaction header")
	}

	i := copy(h.Txid[:], data[:])
	i += copy(h.SourceAdiChain[:], data[i:])
	i += copy(h.SourceChainId[:], data[i:])

	return nil
}
