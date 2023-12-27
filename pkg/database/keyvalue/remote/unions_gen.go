// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package remote

// GENERATED BY go run ./tools/cmd/gen-types. DO NOT EDIT.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

// newCall creates a new call for the specified callType.
func newCall(typ callType) (call, error) {
	switch typ {
	case callTypeBatch:
		return new(batchCall), nil
	case callTypeCommit:
		return new(commitCall), nil
	case callTypeDelete:
		return new(deleteCall), nil
	case callTypeForEach:
		return new(forEachCall), nil
	case callTypeGet:
		return new(getCall), nil
	case callTypePut:
		return new(putCall), nil
	}
	return nil, fmt.Errorf("unknown call %v", typ)
}

// equalCall is used to compare the values of the union
func equalCall(a, b call) bool {
	if a == b {
		return true
	}
	switch a := a.(type) {
	case *batchCall:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*batchCall)
		return ok && a.Equal(b)
	case *commitCall:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*commitCall)
		return ok && a.Equal(b)
	case *deleteCall:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*deleteCall)
		return ok && a.Equal(b)
	case *forEachCall:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*forEachCall)
		return ok && a.Equal(b)
	case *getCall:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*getCall)
		return ok && a.Equal(b)
	case *putCall:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*putCall)
		return ok && a.Equal(b)
	}
	return false
}

// copyCall copies a call.
func copyCall(v call) call {
	switch v := v.(type) {
	case *batchCall:
		return v.Copy()
	case *commitCall:
		return v.Copy()
	case *deleteCall:
		return v.Copy()
	case *forEachCall:
		return v.Copy()
	case *getCall:
		return v.Copy()
	case *putCall:
		return v.Copy()
	default:
		return v.CopyAsInterface().(call)
	}
}

// unmarshalCall unmarshals a call.
func unmarshalCall(data []byte) (call, error) {
	return unmarshalCallFrom(bytes.NewReader(data))
}

// unmarshalCallFrom unmarshals a call.
func unmarshalCallFrom(rd io.Reader) (call, error) {
	reader := encoding.NewReader(rd)

	// Read the type code
	var typ callType
	if !reader.ReadEnum(1, &typ) {
		if reader.IsEmpty() {
			return nil, nil
		}
		return nil, fmt.Errorf("field Type: missing")
	}

	// Create a new call
	v, err := newCall(callType(typ))
	if err != nil {
		return nil, err
	}

	// Unmarshal the rest of the call
	err = v.UnmarshalFieldsFrom(reader)
	if err != nil {
		return nil, err
	}

	return v, nil
}

// unmarshalCallJson unmarshals a call.
func unmarshalCallJSON(data []byte) (call, error) {
	var typ *struct{ Type callType }
	err := json.Unmarshal(data, &typ)
	if err != nil {
		return nil, err
	}

	if typ == nil {
		return nil, nil
	}

	acnt, err := newCall(typ.Type)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, acnt)
	if err != nil {
		return nil, err
	}

	return acnt, nil
}

// newResponse creates a new response for the specified responseType.
func newResponse(typ responseType) (response, error) {
	switch typ {
	case responseTypeBatch:
		return new(batchResponse), nil
	case responseTypeEntry:
		return new(entryResponse), nil
	case responseTypeError:
		return new(errorResponse), nil
	case responseTypeNotFound:
		return new(notFoundResponse), nil
	case responseTypeOk:
		return new(okResponse), nil
	case responseTypeUnsupportedCall:
		return new(unsupportedCallResponse), nil
	case responseTypeValue:
		return new(valueResponse), nil
	}
	return nil, fmt.Errorf("unknown response %v", typ)
}

// equalResponse is used to compare the values of the union
func equalResponse(a, b response) bool {
	if a == b {
		return true
	}
	switch a := a.(type) {
	case *batchResponse:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*batchResponse)
		return ok && a.Equal(b)
	case *entryResponse:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*entryResponse)
		return ok && a.Equal(b)
	case *errorResponse:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*errorResponse)
		return ok && a.Equal(b)
	case *notFoundResponse:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*notFoundResponse)
		return ok && a.Equal(b)
	case *okResponse:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*okResponse)
		return ok && a.Equal(b)
	case *unsupportedCallResponse:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*unsupportedCallResponse)
		return ok && a.Equal(b)
	case *valueResponse:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*valueResponse)
		return ok && a.Equal(b)
	}
	return false
}

// copyResponse copies a response.
func copyResponse(v response) response {
	switch v := v.(type) {
	case *batchResponse:
		return v.Copy()
	case *entryResponse:
		return v.Copy()
	case *errorResponse:
		return v.Copy()
	case *notFoundResponse:
		return v.Copy()
	case *okResponse:
		return v.Copy()
	case *unsupportedCallResponse:
		return v.Copy()
	case *valueResponse:
		return v.Copy()
	default:
		return v.CopyAsInterface().(response)
	}
}

// unmarshalResponse unmarshals a response.
func unmarshalResponse(data []byte) (response, error) {
	return unmarshalResponseFrom(bytes.NewReader(data))
}

// unmarshalResponseFrom unmarshals a response.
func unmarshalResponseFrom(rd io.Reader) (response, error) {
	reader := encoding.NewReader(rd)

	// Read the type code
	var typ responseType
	if !reader.ReadEnum(1, &typ) {
		if reader.IsEmpty() {
			return nil, nil
		}
		return nil, fmt.Errorf("field Type: missing")
	}

	// Create a new response
	v, err := newResponse(responseType(typ))
	if err != nil {
		return nil, err
	}

	// Unmarshal the rest of the response
	err = v.UnmarshalFieldsFrom(reader)
	if err != nil {
		return nil, err
	}

	return v, nil
}

// unmarshalResponseJson unmarshals a response.
func unmarshalResponseJSON(data []byte) (response, error) {
	var typ *struct{ Type responseType }
	err := json.Unmarshal(data, &typ)
	if err != nil {
		return nil, err
	}

	if typ == nil {
		return nil, nil
	}

	acnt, err := newResponse(typ.Type)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, acnt)
	if err != nil {
		return nil, err
	}

	return acnt, nil
}
