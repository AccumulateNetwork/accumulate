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
	case MessageTypeBadSynthetic:
		return new(BadSyntheticMessage), nil
	case MessageTypeBlockAnchor:
		return new(BlockAnchor), nil
	case MessageTypeBlockSummary:
		return new(BlockSummary), nil
	case MessageTypeCreditPayment:
		return new(CreditPayment), nil
	case MessageTypeDidUpdateExecutorVersion:
		return new(DidUpdateExecutorVersion), nil
	case MessageTypeMakeMajorBlock:
		return new(MakeMajorBlock), nil
	case MessageTypeNetworkUpdate:
		return new(NetworkUpdate), nil
	case MessageTypeSecretRelease:
		return new(SecretRelease), nil
	case MessageTypeSequenced:
		return new(SequencedMessage), nil
	case MessageTypeSignature:
		return new(SignatureMessage), nil
	case MessageTypeSignatureRequest:
		return new(SignatureRequest), nil
	case MessageTypeSynthetic:
		return new(SyntheticMessage), nil
	case MessageTypeTransaction:
		return new(TransactionMessage), nil
	}
	return nil, fmt.Errorf("unknown message %v", typ)
}

// EqualMessage is used to compare the values of the union
func EqualMessage(a, b Message) bool {
	if a == b {
		return true
	}
	switch a := a.(type) {
	case *BadSyntheticMessage:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*BadSyntheticMessage)
		return ok && a.Equal(b)
	case *BlockAnchor:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*BlockAnchor)
		return ok && a.Equal(b)
	case *BlockSummary:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*BlockSummary)
		return ok && a.Equal(b)
	case *CreditPayment:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*CreditPayment)
		return ok && a.Equal(b)
	case *DidUpdateExecutorVersion:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*DidUpdateExecutorVersion)
		return ok && a.Equal(b)
	case *MakeMajorBlock:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*MakeMajorBlock)
		return ok && a.Equal(b)
	case *NetworkUpdate:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*NetworkUpdate)
		return ok && a.Equal(b)
	case *SecretRelease:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*SecretRelease)
		return ok && a.Equal(b)
	case *SequencedMessage:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*SequencedMessage)
		return ok && a.Equal(b)
	case *SignatureMessage:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*SignatureMessage)
		return ok && a.Equal(b)
	case *SignatureRequest:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*SignatureRequest)
		return ok && a.Equal(b)
	case *SyntheticMessage:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*SyntheticMessage)
		return ok && a.Equal(b)
	case *TransactionMessage:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*TransactionMessage)
		return ok && a.Equal(b)
	}
	return false
}

// CopyMessage copies a Message.
func CopyMessage(v Message) Message {
	switch v := v.(type) {
	case *BadSyntheticMessage:
		return v.Copy()
	case *BlockAnchor:
		return v.Copy()
	case *BlockSummary:
		return v.Copy()
	case *CreditPayment:
		return v.Copy()
	case *DidUpdateExecutorVersion:
		return v.Copy()
	case *MakeMajorBlock:
		return v.Copy()
	case *NetworkUpdate:
		return v.Copy()
	case *SecretRelease:
		return v.Copy()
	case *SequencedMessage:
		return v.Copy()
	case *SignatureMessage:
		return v.Copy()
	case *SignatureRequest:
		return v.Copy()
	case *SyntheticMessage:
		return v.Copy()
	case *TransactionMessage:
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
		if reader.IsEmpty() {
			return nil, nil
		}
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
