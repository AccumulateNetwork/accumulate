// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package query

// GENERATED BY go run ./tools/cmd/gen-types. DO NOT EDIT.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

// NewRequest creates a new Request for the specified QueryType.
func NewRequest(typ QueryType) (Request, error) {
	switch typ {
	case QueryTypeChainId:
		return new(RequestByChainId), nil
	case QueryTypeTxId:
		return new(RequestByTxId), nil
	case QueryTypeUrl:
		return new(RequestByUrl), nil
	case QueryTypeData:
		return new(RequestDataEntry), nil
	case QueryTypeDataSet:
		return new(RequestDataEntrySet), nil
	case QueryTypeDirectoryUrl:
		return new(RequestDirectory), nil
	case QueryTypeKeyPageIndex:
		return new(RequestKeyPageIndex), nil
	case QueryTypeMajorBlocks:
		return new(RequestMajorBlocks), nil
	case QueryTypeMinorBlocks:
		return new(RequestMinorBlocks), nil
	case QueryTypeSynth:
		return new(RequestSynth), nil
	case QueryTypeTxHistory:
		return new(RequestTxHistory), nil
	case QueryTypeUnknown:
		return new(UnknownRequest), nil
	default:
		return nil, fmt.Errorf("unknown query %v", typ)
	}
}

// EqualRequest is used to compare the values of the union
func EqualRequest(a, b Request) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	switch a := a.(type) {
	case *RequestByChainId:
		b, ok := b.(*RequestByChainId)
		return ok && a.Equal(b)
	case *RequestByTxId:
		b, ok := b.(*RequestByTxId)
		return ok && a.Equal(b)
	case *RequestByUrl:
		b, ok := b.(*RequestByUrl)
		return ok && a.Equal(b)
	case *RequestDataEntry:
		b, ok := b.(*RequestDataEntry)
		return ok && a.Equal(b)
	case *RequestDataEntrySet:
		b, ok := b.(*RequestDataEntrySet)
		return ok && a.Equal(b)
	case *RequestDirectory:
		b, ok := b.(*RequestDirectory)
		return ok && a.Equal(b)
	case *RequestKeyPageIndex:
		b, ok := b.(*RequestKeyPageIndex)
		return ok && a.Equal(b)
	case *RequestMajorBlocks:
		b, ok := b.(*RequestMajorBlocks)
		return ok && a.Equal(b)
	case *RequestMinorBlocks:
		b, ok := b.(*RequestMinorBlocks)
		return ok && a.Equal(b)
	case *RequestSynth:
		b, ok := b.(*RequestSynth)
		return ok && a.Equal(b)
	case *RequestTxHistory:
		b, ok := b.(*RequestTxHistory)
		return ok && a.Equal(b)
	case *UnknownRequest:
		b, ok := b.(*UnknownRequest)
		return ok && a.Equal(b)
	default:
		return false
	}
}

// CopyRequest copies a Request.
func CopyRequest(v Request) Request {
	switch v := v.(type) {
	case *RequestByChainId:
		return v.Copy()
	case *RequestByTxId:
		return v.Copy()
	case *RequestByUrl:
		return v.Copy()
	case *RequestDataEntry:
		return v.Copy()
	case *RequestDataEntrySet:
		return v.Copy()
	case *RequestDirectory:
		return v.Copy()
	case *RequestKeyPageIndex:
		return v.Copy()
	case *RequestMajorBlocks:
		return v.Copy()
	case *RequestMinorBlocks:
		return v.Copy()
	case *RequestSynth:
		return v.Copy()
	case *RequestTxHistory:
		return v.Copy()
	case *UnknownRequest:
		return v.Copy()
	default:
		return v.CopyAsInterface().(Request)
	}
}

// UnmarshalRequest unmarshals a Request.
func UnmarshalRequest(data []byte) (Request, error) {
	return UnmarshalRequestFrom(bytes.NewReader(data))
}

// UnmarshalRequestFrom unmarshals a Request.
func UnmarshalRequestFrom(rd io.Reader) (Request, error) {
	reader := encoding.NewReader(rd)

	// Read the type code
	var typ QueryType
	if !reader.ReadEnum(1, &typ) {
		return nil, fmt.Errorf("field Type: missing")
	}

	// Create a new request
	v, err := NewRequest(QueryType(typ))
	if err != nil {
		return nil, err
	}

	// Unmarshal the rest of the request
	err = v.UnmarshalFieldsFrom(reader)
	if err != nil {
		return nil, err
	}

	return v, nil
}

// UnmarshalRequestJson unmarshals a Request.
func UnmarshalRequestJSON(data []byte) (Request, error) {
	var typ *struct{ Type QueryType }
	err := json.Unmarshal(data, &typ)
	if err != nil {
		return nil, err
	}

	if typ == nil {
		return nil, nil
	}

	acnt, err := NewRequest(typ.Type)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, acnt)
	if err != nil {
		return nil, err
	}

	return acnt, nil
}
