// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

// GENERATED BY go run ./tools/cmd/gen-types. DO NOT EDIT.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

// NewMessage creates a new Message for the specified messageType.
func NewMessage(typ messageType) (Message, error) {
	switch typ {
	case messageTypeEnvelopeSubmitted:
		return new(EnvelopeSubmitted), nil
	case messageTypeExecutedBlock:
		return new(ExecutedBlock), nil
	case messageTypeStartBlock:
		return new(StartBlock), nil
	case messageTypeSubmitEnvelope:
		return new(SubmitEnvelope), nil
	case messageTypeAcceptBlockProposal:
		return new(acceptBlockProposal), nil
	case messageTypeAcceptedSubmission:
		return new(acceptedSubmission), nil
	case messageTypeCommittedBlock:
		return new(committedBlock), nil
	case messageTypeFinalizedBlock:
		return new(finalizedBlock), nil
	case messageTypeProposeBlock:
		return new(proposeBlock), nil
	case messageTypeProposeLeader:
		return new(proposeLeader), nil
	}
	return nil, fmt.Errorf("unknown message %v", typ)
}

// EqualMessage is used to compare the values of the union
func EqualMessage(a, b Message) bool {
	if a == b {
		return true
	}
	switch a := a.(type) {
	case *EnvelopeSubmitted:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*EnvelopeSubmitted)
		return ok && a.Equal(b)
	case *ExecutedBlock:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*ExecutedBlock)
		return ok && a.Equal(b)
	case *StartBlock:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*StartBlock)
		return ok && a.Equal(b)
	case *SubmitEnvelope:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*SubmitEnvelope)
		return ok && a.Equal(b)
	case *acceptBlockProposal:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*acceptBlockProposal)
		return ok && a.Equal(b)
	case *acceptedSubmission:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*acceptedSubmission)
		return ok && a.Equal(b)
	case *committedBlock:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*committedBlock)
		return ok && a.Equal(b)
	case *finalizedBlock:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*finalizedBlock)
		return ok && a.Equal(b)
	case *proposeBlock:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*proposeBlock)
		return ok && a.Equal(b)
	case *proposeLeader:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*proposeLeader)
		return ok && a.Equal(b)
	}
	return false
}

// CopyMessage copies a Message.
func CopyMessage(v Message) Message {
	switch v := v.(type) {
	case *EnvelopeSubmitted:
		return v.Copy()
	case *ExecutedBlock:
		return v.Copy()
	case *StartBlock:
		return v.Copy()
	case *SubmitEnvelope:
		return v.Copy()
	case *acceptBlockProposal:
		return v.Copy()
	case *acceptedSubmission:
		return v.Copy()
	case *committedBlock:
		return v.Copy()
	case *finalizedBlock:
		return v.Copy()
	case *proposeBlock:
		return v.Copy()
	case *proposeLeader:
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
	var typ messageType
	if !reader.ReadEnum(1, &typ) {
		if reader.IsEmpty() {
			return nil, nil
		}
		return nil, fmt.Errorf("field Type: missing")
	}

	// Create a new message
	v, err := NewMessage(messageType(typ))
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
	var typ *struct{ Type messageType }
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
