package query

import (
	"bytes"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/internal/encoding"
	"github.com/AccumulateNetwork/accumulate/types"
)

type RequestByUrl struct {
	Url types.String
}

type RequestDirectory struct {
	RequestByUrl
	Start        uint64
	Limit        uint64
	ExpandChains bool
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

func (*RequestDirectory) Type() types.QueryType { return types.QueryTypeDirectoryUrl }

func (r *RequestDirectory) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer
	binary, err := r.Url.MarshalBinary()
	if err != nil {
		return nil, err
	}
	buffer.Write(binary)
	buffer.Write(encoding.UvarintMarshalBinary(r.Start))
	buffer.Write(encoding.UvarintMarshalBinary(r.Limit))
	buffer.Write(encoding.BoolMarshalBinary(r.ExpandChains))
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
	data = data[r.Url.Size(nil):]

	if start, err := encoding.UvarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Start: %w", err)
	} else {
		r.Start = start
	}
	data = data[encoding.UvarintBinarySize(r.Start):]

	if limit, err := encoding.UvarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Limit: %w", err)
	} else {
		r.Limit = limit
	}
	data = data[encoding.UvarintBinarySize(r.Limit):]

	if expand, err := encoding.BoolUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding ExpandChains: %w", err)
	} else {
		r.ExpandChains = expand
	}
	return err
}
