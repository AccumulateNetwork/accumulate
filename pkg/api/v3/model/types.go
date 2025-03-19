// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package model

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// Error represents an error response
type Error struct {
	Error *errors.Error
}

// SequenceOptions represents options for sequence requests
type SequenceOptions struct {
	Start  uint64
	Count  uint64
	Values bool
}

// Copy creates a deep copy of SequenceOptions
func (s *SequenceOptions) Copy() *SequenceOptions {
	if s == nil {
		return nil
	}
	return &SequenceOptions{
		Start:  s.Start,
		Count:  s.Count,
		Values: s.Values,
	}
}

// MarshalBinary implements encoding.BinaryMarshaler
func (s *SequenceOptions) MarshalBinary() ([]byte, error) {
	// Use encoding/binary to marshal the fields
	data := make([]byte, 17) // 8 bytes for Start + 8 bytes for Count + 1 byte for Values
	binary.BigEndian.PutUint64(data[0:8], s.Start)
	binary.BigEndian.PutUint64(data[8:16], s.Count)
	if s.Values {
		data[16] = 1
	}
	return data, nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler
func (s *SequenceOptions) UnmarshalBinary(data []byte) error {
	if len(data) != 17 {
		return fmt.Errorf("invalid data length for SequenceOptions: expected 17, got %d", len(data))
	}
	s.Start = binary.BigEndian.Uint64(data[0:8])
	s.Count = binary.BigEndian.Uint64(data[8:16])
	s.Values = data[16] == 1
	return nil
}

// MessageRecord represents a record containing a message
type MessageRecord struct {
	ID      uint64
	Message []byte
	Meta    json.RawMessage
}

// Copy creates a deep copy of MessageRecord
func (r *MessageRecord) Copy() *MessageRecord {
	if r == nil {
		return nil
	}
	msg := make([]byte, len(r.Message))
	copy(msg, r.Message)
	meta := make([]byte, len(r.Meta))
	copy(meta, r.Meta)
	return &MessageRecord{
		ID:      r.ID,
		Message: msg,
		Meta:    meta,
	}
}

// MarshalBinary implements encoding.BinaryMarshaler
func (r *MessageRecord) MarshalBinary() ([]byte, error) {
	if r == nil {
		return nil, nil
	}
	// Simple encoding: [8 bytes ID][4 bytes msgLen][msgLen bytes][4 bytes metaLen][metaLen bytes]
	msgLen := len(r.Message)
	metaLen := len(r.Meta)
	data := make([]byte, 16+msgLen+metaLen)
	
	// Write ID
	data[0] = byte(r.ID >> 56)
	data[1] = byte(r.ID >> 48)
	data[2] = byte(r.ID >> 40)
	data[3] = byte(r.ID >> 32)
	data[4] = byte(r.ID >> 24)
	data[5] = byte(r.ID >> 16)
	data[6] = byte(r.ID >> 8)
	data[7] = byte(r.ID)
	
	// Write message length
	data[8] = byte(msgLen >> 24)
	data[9] = byte(msgLen >> 16)
	data[10] = byte(msgLen >> 8)
	data[11] = byte(msgLen)
	
	// Write message
	copy(data[12:], r.Message)
	
	// Write meta length
	offset := 12 + msgLen
	data[offset] = byte(metaLen >> 24)
	data[offset+1] = byte(metaLen >> 16)
	data[offset+2] = byte(metaLen >> 8)
	data[offset+3] = byte(metaLen)
	
	// Write meta
	copy(data[offset+4:], r.Meta)
	
	return data, nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler
func (r *MessageRecord) UnmarshalBinary(data []byte) error {
	if len(data) < 12 {
		return nil
	}
	
	// Read ID
	r.ID = uint64(data[0])<<56 | uint64(data[1])<<48 | uint64(data[2])<<40 | uint64(data[3])<<32 |
		uint64(data[4])<<24 | uint64(data[5])<<16 | uint64(data[6])<<8 | uint64(data[7])
	
	// Read message length
	msgLen := int(data[8])<<24 | int(data[9])<<16 | int(data[10])<<8 | int(data[11])
	if len(data) < 16+msgLen {
		return nil
	}
	
	// Read message
	r.Message = make([]byte, msgLen)
	copy(r.Message, data[12:12+msgLen])
	
	// Read meta length
	offset := 12 + msgLen
	metaLen := int(data[offset])<<24 | int(data[offset+1])<<16 | int(data[offset+2])<<8 | int(data[offset+3])
	if len(data) < offset+4+metaLen {
		return nil
	}
	
	// Read meta
	r.Meta = make([]byte, metaLen)
	copy(r.Meta, data[offset+4:offset+4+metaLen])
	
	return nil
}

// Message type constants
const (
	TypeErrorResponse uint32 = iota + 1
	TypePrivateSequenceRequest
	TypePrivateSequenceResponse
)
