// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

// GENERATED BY go run ./tools/cmd/gen-enum. DO NOT EDIT.

import (
	"encoding/json"
	"fmt"
	"strings"
)

// EventTypeServiceRegistered .
const EventTypeServiceRegistered EventType = 1

// GetEnumValue returns the value of the Event Type
func (v EventType) GetEnumValue() uint64 { return uint64(v) }

// SetEnumValue sets the value. SetEnumValue returns false if the value is invalid.
func (v *EventType) SetEnumValue(id uint64) bool {
	u := EventType(id)
	switch u {
	case EventTypeServiceRegistered:
		*v = u
		return true
	default:
		return false
	}
}

// String returns the name of the Event Type.
func (v EventType) String() string {
	switch v {
	case EventTypeServiceRegistered:
		return "serviceRegistered"
	default:
		return fmt.Sprintf("EventType:%d", v)
	}
}

// EventTypeByName returns the named Event Type.
func EventTypeByName(name string) (EventType, bool) {
	switch strings.ToLower(name) {
	case "serviceregistered":
		return EventTypeServiceRegistered, true
	default:
		return 0, false
	}
}

// MarshalJSON marshals the Event Type to JSON as a string.
func (v EventType) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

// UnmarshalJSON unmarshals the Event Type from JSON as a string.
func (v *EventType) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	var ok bool
	*v, ok = EventTypeByName(s)
	if !ok || strings.ContainsRune(v.String(), ':') {
		return fmt.Errorf("invalid Event Type %q", s)
	}
	return nil
}
