package synthetic

import (
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
)

type AdiStateCreate struct {
	Header
	api.ADI
}

func NewAdiStateCreate(adi string, keyHash *types.Bytes32) *AdiStateCreate {
	ctas := &AdiStateCreate{}
	ctas.ADI.URL = types.String(adi)
	ctas.PublicKeyHash = *keyHash

	return ctas
}

//
//func (is *AdiStateCreate) MarshalJSON() ([]byte, error) {
//
//
//	data, err := json.Marshal(&is.Header)
//	if err != nil {
//		return nil, fmt.Errorf("unable to marshal AdiStateCreate header %v", err)
//	}
//
//	return json.Marshal(&is.adiState)
//}
//
//func (is *AdiStateCreate) UnmarshalJSON(data []byte) error {
//	return json.Unmarshal(data,&is.adiState)
//}
