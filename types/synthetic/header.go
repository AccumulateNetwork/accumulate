package synthetic

import (
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
)

type Header struct {
	Txid           types.Bytes32 `json:"txid"`
	SourceIdentity types.Bytes32 `json:"source-identity"`
	SourceChainId  types.Bytes32 `json:"source-chain-id"`
}

const HeaderLen = 32 * 3

func (h *Header) MarshalBinary() ([]byte, error) {
	data := make([]byte, HeaderLen)
	i := copy(data[:], h.Txid[:])
	i += copy(data[i:], h.SourceIdentity[:])
	i += copy(data[i:], h.SourceChainId[:])
	return data, nil
}

func (h *Header) UnmarshalBinary(data []byte) error {
	if HeaderLen > len(data) {
		return fmt.Errorf("Insufficient data to unmarshal synthetic transaction header")
	}

	i := copy(h.Txid[:], data[:])
	i += copy(h.SourceIdentity[:], data[i:])
	i += copy(h.SourceChainId[:], data[i:])

	return nil
}

//
//func (h *Header) UnmarshalJSON(data []byte) error {
//	var result map[string]interface{}
//	err := json.Unmarshal(data,&result)
//	if err != nil {
//		return err
//	}
//	d := json.NewDecoder(strings.NewReader(string(data)))
//	d.DisallowUnknownFields()
//	d.DisallowUnknownFields()
//
//	if err := d.Decode(&a)
//	h.Txid
//	fmt.Sscanf()Scanf("%x",)result["txid"].(map[string]interface{}).(string)
//
//	return nil
//}
