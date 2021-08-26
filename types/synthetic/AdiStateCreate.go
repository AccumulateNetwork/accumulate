package synthetic

import (
	"github.com/AccumulateNetwork/accumulated/types/state"
)

type AdiStateCreate struct {
	*state.AdiState `json:"adiState"`
	*Header `json:"header"`
}

func NewIdentityStateCreate(adi string) *AdiStateCreate {
	ctas := &AdiStateCreate{}
	ctas.Header = &Header{}
	ctas.AdiState = state.NewIdentityState(adi)
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
