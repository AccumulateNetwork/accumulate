// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package messaging

// GENERATED BY go run ./tools/cmd/gen-types. DO NOT EDIT.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

// NewMessage creates a new Message for the specified MessageType.
func NewMessage(typ MessageType) (Message, error) {
	switch typ {
	case MessageTypeSyntheticTransaction:
		return new(SyntheticTransaction), nil
	case MessageTypeUserSignature:
		return new(UserSignature), nil
	case MessageTypeUserTransaction:
		return new(UserTransaction), nil
	default:
		return nil, fmt.Errorf("unknown message %v", typ)
	}
}

// EqualMessage is used to compare the values of the union
func EqualMessage(a, b Message) bool {
	if a == b {
		return true
	}
	switch a := a.(type) {
	case *SyntheticTransaction:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*SyntheticTransaction)
		return ok && a.Equal(b)
	case *UserSignature:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*UserSignature)
		return ok && a.Equal(b)
	case *UserTransaction:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*UserTransaction)
		return ok && a.Equal(b)
	default:
		return false
	}
}

// CopyMessage copies a Message.
func CopyMessage(v Message) Message {
	switch v := v.(type) {
	case *SyntheticTransaction:
		return v.Copy()
	case *UserSignature:
		return v.Copy()
	case *UserTransaction:
		return v.Copy()
	default:
		return v.CopyAsInterface().(Message)
	}
}

// UnmarshalMessage unmarshals a Message.
func UnmarshalMessage(data []byte) (Message, error) {
	return UnmarshalMessageFrom(bytes.NewReader(data))
}

// UnmarshalMessageFrom unmarshals a Message.
func UnmarshalMessageFrom(rd io.Reader) (Message, error) {
	reader := encoding.NewReader(rd)

	// Read the type code
	var typ MessageType
	if !reader.ReadEnum(1, &typ) {
		return nil, fmt.Errorf("field Type: missing")
	}

	// Create a new message
	v, err := NewMessage(MessageType(typ))
	if err != nil {
		return nil, err
	}

	// Unmarshal the rest of the message
	err = v.UnmarshalFieldsFrom(reader)
	if err != nil {
		return nil, err
	}

	return v, nil
}

// UnmarshalMessageJson unmarshals a Message.
func UnmarshalMessageJSON(data []byte) (Message, error) {
	var typ *struct{ Type MessageType }
	err := json.Unmarshal(data, &typ)
	if err != nil {
		return nil, err
	}

	if typ == nil {
		return nil, nil
	}

	acnt, err := NewMessage(typ.Type)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, acnt)
	if err != nil {
		return nil, err
	}

	return acnt, nil
}
