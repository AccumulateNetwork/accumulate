package query

import (
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

type RequestByUrl struct {
	Url types.String
}

type ResponseByUrl struct {
	StateObject state.Object
}

func (r *RequestByUrl) MarshalBinary() ([]byte, error) {
	return r.Url.MarshalBinary()
}

func (r *RequestByUrl) UnmarshalBinary(data []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("error unmarshaling RequestByUrl data %v", r)
		}
	}()
	return r.Url.UnmarshalBinary(data)
}

func (r *ResponseByUrl) MarshalBinary() ([]byte, error) {
	return r.StateObject.MarshalBinary()
}

func (r *ResponseByUrl) UnmarshalBinary(data []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("error unmarshaling ResponseByUrl data %v", r)
		}
	}()

	return r.StateObject.UnmarshalBinary(data)
}
