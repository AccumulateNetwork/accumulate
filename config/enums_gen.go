package config

// GENERATED BY go run ./tools/cmd/gen-enum. DO NOT EDIT.

import (
	"encoding/json"
	"fmt"
	"strings"
)

// NetworkTypeDirectory .
const NetworkTypeDirectory NetworkType = 1

// NetworkTypeBlockValidator .
const NetworkTypeBlockValidator NetworkType = 2

// NodeTypeValidator .
const NodeTypeValidator NodeType = 1

// NodeTypeFollower .
const NodeTypeFollower NodeType = 2

// PortOffsetTendermintP2P .
const PortOffsetTendermintP2P PortOffset = 0

// PortOffsetTendermintRpc .
const PortOffsetTendermintRpc PortOffset = 1

// PortOffsetWebsite .
const PortOffsetWebsite PortOffset = 2

// PortOffsetPrometheus .
const PortOffsetPrometheus PortOffset = 3

// PortOffsetAccumulateApi .
const PortOffsetAccumulateApi PortOffset = 4

// PortOffsetTmRpcGrpc .
const PortOffsetTmRpcGrpc PortOffset = 5

// GetEnumValue returns the value of the Network Type
func (v NetworkType) GetEnumValue() uint64 { return uint64(v) }

// SetEnumValue sets the value. SetEnumValue returns false if the value is invalid.
func (v *NetworkType) SetEnumValue(id uint64) bool {
	u := NetworkType(id)
	switch u {
	case NetworkTypeDirectory, NetworkTypeBlockValidator:
		*v = u
		return true
	default:
		return false
	}
}

// String returns the name of the Network Type.
func (v NetworkType) String() string {
	switch v {
	case NetworkTypeDirectory:
		return "directory"
	case NetworkTypeBlockValidator:
		return "blockValidator"
	default:
		return fmt.Sprintf("NetworkType:%d", v)
	}
}

// NetworkTypeByName returns the named Network Type.
func NetworkTypeByName(name string) (NetworkType, bool) {
	switch strings.ToLower(name) {
	case "directory":
		return NetworkTypeDirectory, true
	case "blockvalidator":
		return NetworkTypeBlockValidator, true
	case "block-validator":
		return NetworkTypeBlockValidator, true
	default:
		return 0, false
	}
}

// MarshalJSON marshals the Network Type to JSON as a string.
func (v NetworkType) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

// UnmarshalJSON unmarshals the Network Type from JSON as a string.
func (v *NetworkType) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	var ok bool
	*v, ok = NetworkTypeByName(s)
	if !ok || strings.ContainsRune(v.String(), ':') {
		return fmt.Errorf("invalid Network Type %q", s)
	}
	return nil
}

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
	case PortOffsetTendermintP2P, PortOffsetTendermintRpc, PortOffsetWebsite, PortOffsetPrometheus, PortOffsetAccumulateApi, PortOffsetTmRpcGrpc:
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
	case PortOffsetWebsite:
		return "website"
	case PortOffsetPrometheus:
		return "prometheus"
	case PortOffsetAccumulateApi:
		return "accumulateApi"
	case PortOffsetTmRpcGrpc:
		return "tmRpcGrpc"
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
	case "website":
		return PortOffsetWebsite, true
	case "prometheus":
		return PortOffsetPrometheus, true
	case "accumulateapi":
		return PortOffsetAccumulateApi, true
	case "tmrpcgrpc":
		return PortOffsetTmRpcGrpc, true
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
