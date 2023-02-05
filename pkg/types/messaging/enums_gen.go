// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package messaging

// GENERATED BY go run ./tools/cmd/gen-enum. DO NOT EDIT.

import (
	"encoding/json"
	"fmt"
	"strings"
)

// MessageTypeUserTransaction is a transaction from a user.
const MessageTypeUserTransaction MessageType = 1

// MessageTypeUserSignature is a key signature from a user.
const MessageTypeUserSignature MessageType = 2

// MessageTypeSynthetic is a message produced by the protocol, requiring proof.
const MessageTypeSynthetic MessageType = 3

// MessageTypeValidatorSignature is a key signature from a validator.
const MessageTypeValidatorSignature MessageType = 4

// MessageTypeSequenced is a message that is part of a sequence.
const MessageTypeSequenced MessageType = 5

// GetEnumValue returns the value of the Message Type
func (v MessageType) GetEnumValue() uint64 { return uint64(v) }

// SetEnumValue sets the value. SetEnumValue returns false if the value is invalid.
func (v *MessageType) SetEnumValue(id uint64) bool {
	u := MessageType(id)
	switch u {
	case MessageTypeUserTransaction, MessageTypeUserSignature, MessageTypeSynthetic, MessageTypeValidatorSignature, MessageTypeSequenced:
		*v = u
		return true
	default:
		return false
	}
}

// String returns the name of the Message Type.
func (v MessageType) String() string {
	switch v {
	case MessageTypeUserTransaction:
		return "userTransaction"
	case MessageTypeUserSignature:
		return "userSignature"
	case MessageTypeSynthetic:
		return "synthetic"
	case MessageTypeValidatorSignature:
		return "validatorSignature"
	case MessageTypeSequenced:
		return "sequenced"
	default:
		return fmt.Sprintf("MessageType:%d", v)
	}
}

// MessageTypeByName returns the named Message Type.
func MessageTypeByName(name string) (MessageType, bool) {
	switch strings.ToLower(name) {
	case "usertransaction":
		return MessageTypeUserTransaction, true
	case "usersignature":
		return MessageTypeUserSignature, true
	case "synthetic":
		return MessageTypeSynthetic, true
	case "validatorsignature":
		return MessageTypeValidatorSignature, true
	case "sequenced":
		return MessageTypeSequenced, true
	default:
		return 0, false
	}
}

// MarshalJSON marshals the Message Type to JSON as a string.
func (v MessageType) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

// UnmarshalJSON unmarshals the Message Type from JSON as a string.
func (v *MessageType) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	var ok bool
	*v, ok = MessageTypeByName(s)
	if !ok || strings.ContainsRune(v.String(), ':') {
		return fmt.Errorf("invalid Message Type %q", s)
	}
	return nil
}
