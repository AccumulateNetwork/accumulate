// Copyright 2024 The Accumulate Authors
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

// MessageTypeTransaction is a transaction.
const MessageTypeTransaction MessageType = 1

// MessageTypeSignature is a signature.
const MessageTypeSignature MessageType = 2

// MessageTypeBadSynthetic is deprecated.
const MessageTypeBadSynthetic MessageType = 3

// MessageTypeBlockAnchor is a block anchor signed by validator.
const MessageTypeBlockAnchor MessageType = 4

// MessageTypeSequenced is a message that is part of a sequence.
const MessageTypeSequenced MessageType = 5

// MessageTypeSignatureRequest is a request for additional signatures.
const MessageTypeSignatureRequest MessageType = 6

// MessageTypeCreditPayment is a payment of credits towards a transaction's fee.
const MessageTypeCreditPayment MessageType = 7

// MessageTypeBlockSummary is a summary of a block.
const MessageTypeBlockSummary MessageType = 8

// MessageTypeSynthetic is a message produced by the protocol, requiring proof.
const MessageTypeSynthetic MessageType = 9

// MessageTypeNetworkUpdate is an update to network parameters.
const MessageTypeNetworkUpdate MessageType = 10

// MessageTypeMakeMajorBlock triggers a major block.
const MessageTypeMakeMajorBlock MessageType = 11

// MessageTypeDidUpdateExecutorVersion notifies the DN that a BVN updated the executor version.
const MessageTypeDidUpdateExecutorVersion MessageType = 12

// GetEnumValue returns the value of the Message Type
func (v MessageType) GetEnumValue() uint64 { return uint64(v) }

// SetEnumValue sets the value. SetEnumValue returns false if the value is invalid.
func (v *MessageType) SetEnumValue(id uint64) bool {
	u := MessageType(id)
	switch u {
	case MessageTypeTransaction, MessageTypeSignature, MessageTypeBadSynthetic, MessageTypeBlockAnchor, MessageTypeSequenced, MessageTypeSignatureRequest, MessageTypeCreditPayment, MessageTypeBlockSummary, MessageTypeSynthetic, MessageTypeNetworkUpdate, MessageTypeMakeMajorBlock, MessageTypeDidUpdateExecutorVersion:
		*v = u
		return true
	}
	return false
}

// String returns the name of the Message Type.
func (v MessageType) String() string {
	switch v {
	case MessageTypeTransaction:
		return "transaction"
	case MessageTypeSignature:
		return "signature"
	case MessageTypeBadSynthetic:
		return "badSynthetic"
	case MessageTypeBlockAnchor:
		return "blockAnchor"
	case MessageTypeSequenced:
		return "sequenced"
	case MessageTypeSignatureRequest:
		return "signatureRequest"
	case MessageTypeCreditPayment:
		return "creditPayment"
	case MessageTypeBlockSummary:
		return "blockSummary"
	case MessageTypeSynthetic:
		return "synthetic"
	case MessageTypeNetworkUpdate:
		return "networkUpdate"
	case MessageTypeMakeMajorBlock:
		return "makeMajorBlock"
	case MessageTypeDidUpdateExecutorVersion:
		return "didUpdateExecutorVersion"
	}
	return fmt.Sprintf("MessageType:%d", v)
}

// MessageTypeByName returns the named Message Type.
func MessageTypeByName(name string) (MessageType, bool) {
	switch strings.ToLower(name) {
	case "transaction":
		return MessageTypeTransaction, true
	case "signature":
		return MessageTypeSignature, true
	case "badsynthetic":
		return MessageTypeBadSynthetic, true
	case "blockanchor":
		return MessageTypeBlockAnchor, true
	case "sequenced":
		return MessageTypeSequenced, true
	case "signaturerequest":
		return MessageTypeSignatureRequest, true
	case "creditpayment":
		return MessageTypeCreditPayment, true
	case "blocksummary":
		return MessageTypeBlockSummary, true
	case "synthetic":
		return MessageTypeSynthetic, true
	case "networkupdate":
		return MessageTypeNetworkUpdate, true
	case "makemajorblock":
		return MessageTypeMakeMajorBlock, true
	case "didupdateexecutorversion":
		return MessageTypeDidUpdateExecutorVersion, true
	}
	return 0, false
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
