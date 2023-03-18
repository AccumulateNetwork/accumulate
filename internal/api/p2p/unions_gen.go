// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

// GENERATED BY go run ./tools/cmd/gen-types. DO NOT EDIT.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

// NewEvent creates a new Event for the specified EventType.
func NewEvent(typ EventType) (Event, error) {
	switch typ {
	case EventTypeServiceRegistered:
		return new(ServiceRegisteredEvent), nil
	default:
		return nil, fmt.Errorf("unknown event %v", typ)
	}
}

// EqualEvent is used to compare the values of the union
func EqualEvent(a, b Event) bool {
	if a == b {
		return true
	}
	switch a := a.(type) {
	case *ServiceRegisteredEvent:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*ServiceRegisteredEvent)
		return ok && a.Equal(b)
	default:
		return false
	}
}

// CopyEvent copies a Event.
func CopyEvent(v Event) Event {
	switch v := v.(type) {
	case *ServiceRegisteredEvent:
		return v.Copy()
	default:
		return v.CopyAsInterface().(Event)
	}
}

// UnmarshalEvent unmarshals a Event.
func UnmarshalEvent(data []byte) (Event, error) {
	return UnmarshalEventFrom(bytes.NewReader(data))
}

// UnmarshalEventFrom unmarshals a Event.
func UnmarshalEventFrom(rd io.Reader) (Event, error) {
	reader := encoding.NewReader(rd)

	// Read the type code
	var typ EventType
	if !reader.ReadEnum(1, &typ) {
		if reader.IsEmpty() {
			return nil, nil
		}
		return nil, fmt.Errorf("field Type: missing")
	}

	// Create a new event
	v, err := NewEvent(EventType(typ))
	if err != nil {
		return nil, err
	}

	// Unmarshal the rest of the event
	err = v.UnmarshalFieldsFrom(reader)
	if err != nil {
		return nil, err
	}

	return v, nil
}

// UnmarshalEventJson unmarshals a Event.
func UnmarshalEventJSON(data []byte) (Event, error) {
	var typ *struct{ Type EventType }
	err := json.Unmarshal(data, &typ)
	if err != nil {
		return nil, err
	}

	if typ == nil {
		return nil, nil
	}

	acnt, err := NewEvent(typ.Type)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, acnt)
	if err != nil {
		return nil, err
	}

	return acnt, nil
}
