// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package config

// GENERATED BY go run ./tools/cmd/gen-enum. DO NOT EDIT.

import (
	"encoding/json"
	"fmt"
	"strings"
)

// NodeTypeValidator .
const NodeTypeValidator NodeType = 1

// NodeTypeFollower .
const NodeTypeFollower NodeType = 2

// PortOffsetTendermintP2P .
const PortOffsetTendermintP2P PortOffset = 0

// PortOffsetTendermintRpc .
const PortOffsetTendermintRpc PortOffset = 1

// PortOffsetAccumulateP2P .
const PortOffsetAccumulateP2P PortOffset = 2

// PortOffsetPrometheus .
const PortOffsetPrometheus PortOffset = 3

// PortOffsetAccumulateApi .
const PortOffsetAccumulateApi PortOffset = 4

// GetEnumValue returns the value of the Node Type
func (v NodeType) GetEnumValue() uint64 { return uint64(v) }

// SetEnumValue sets the value. SetEnumValue returns false if the value is invalid.
func (v *NodeType) SetEnumValue(id uint64) bool {
	u := NodeType(id)
	switch u {
	case NodeTypeValidator, NodeTypeFollower:
		*v = u
		return true
	default:
		return false
	}
}

// String returns the name of the Node Type.
func (v NodeType) String() string {
	switch v {
	case NodeTypeValidator:
		return "validator"
	case NodeTypeFollower:
		return "follower"
	default:
		return fmt.Sprintf("NodeType:%d", v)
	}
}

// NodeTypeByName returns the named Node Type.
func NodeTypeByName(name string) (NodeType, bool) {
	switch strings.ToLower(name) {
	case "validator":
		return NodeTypeValidator, true
	case "follower":
		return NodeTypeFollower, true
	default:
		return 0, false
	}
}

// MarshalJSON marshals the Node Type to JSON as a string.
func (v NodeType) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

// UnmarshalJSON unmarshals the Node Type from JSON as a string.
func (v *NodeType) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	var ok bool
	*v, ok = NodeTypeByName(s)
	if !ok || strings.ContainsRune(v.String(), ':') {
		return fmt.Errorf("invalid Node Type %q", s)
	}
	return nil
}

// GetEnumValue returns the value of the Port Offset
func (v PortOffset) GetEnumValue() uint64 { return uint64(v) }

// SetEnumValue sets the value. SetEnumValue returns false if the value is invalid.
func (v *PortOffset) SetEnumValue(id uint64) bool {
	u := PortOffset(id)
	switch u {
	case PortOffsetTendermintP2P, PortOffsetTendermintRpc, PortOffsetAccumulateP2P, PortOffsetPrometheus, PortOffsetAccumulateApi:
		*v = u
		return true
	default:
		return false
	}
}

// String returns the name of the Port Offset.
func (v PortOffset) String() string {
	switch v {
	case PortOffsetTendermintP2P:
		return "tendermintP2P"
	case PortOffsetTendermintRpc:
		return "tendermintRpc"
	case PortOffsetAccumulateP2P:
		return "accumulateP2P"
	case PortOffsetPrometheus:
		return "prometheus"
	case PortOffsetAccumulateApi:
		return "accumulateApi"
	default:
		return fmt.Sprintf("PortOffset:%d", v)
	}
}

// PortOffsetByName returns the named Port Offset.
func PortOffsetByName(name string) (PortOffset, bool) {
	switch strings.ToLower(name) {
	case "tendermintp2p":
		return PortOffsetTendermintP2P, true
	case "tendermintrpc":
		return PortOffsetTendermintRpc, true
	case "accumulatep2p":
		return PortOffsetAccumulateP2P, true
	case "prometheus":
		return PortOffsetPrometheus, true
	case "accumulateapi":
		return PortOffsetAccumulateApi, true
	default:
		return 0, false
	}
}

// MarshalJSON marshals the Port Offset to JSON as a string.
func (v PortOffset) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

// UnmarshalJSON unmarshals the Port Offset from JSON as a string.
func (v *PortOffset) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	var ok bool
	*v, ok = PortOffsetByName(s)
	if !ok || strings.ContainsRune(v.String(), ':') {
		return fmt.Errorf("invalid Port Offset %q", s)
	}
	return nil
}