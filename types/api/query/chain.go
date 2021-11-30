package query

import (
	"bytes"
	"fmt"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

type RequestByUrl struct {
	Url types.String
}

type ResponseByUrl struct {
	state.Object
}

type RequestDirectory struct {
	RequestByUrl
	ExpandChains types.Bool
}

type ResponseDirectory struct {
	state.Object
}

func (*RequestByUrl) Type() types.QueryType { return types.QueryTypeUrl }

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
	return r.Object.MarshalBinary()
}

func (r *ResponseByUrl) UnmarshalBinary(data []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("error unmarshaling ResponseByUrl data %v", r)
		}
	}()

	return r.Object.UnmarshalBinary(data)
}

func (r *RequestDirectory) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer
	binary, err := r.Url.MarshalBinary()
	if err != nil {
		return nil, err
	}
	buffer.Write(binary)
	buffer.Write(r.ExpandChains.MarshalBinary())
	return buffer.Bytes(), nil
}

func (r *RequestDirectory) UnmarshalBinary(data []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("error unmarshaling RequestDirectory data %v", r)
		}
	}()
	err = r.Url.UnmarshalBinary(data)
	if err != nil {
		return err
	}
	l := r.Url.Size(nil)
	err = r.ExpandChains.UnmarshalBinary(data[l:])
	return err
}

func (r *ResponseDirectory) MarshalBinary() ([]byte, error) {
	return r.Object.MarshalBinary()
}

func (r *ResponseDirectory) UnmarshalBinary(data []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("error unmarshaling ResponseDirectory data %v", r)
		}
	}()

	return r.Object.UnmarshalBinary(data)
}
