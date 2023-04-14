// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

// GENERATED BY go run ./tools/cmd/gen-enum. DO NOT EDIT.

import (
	"encoding/json"
	"fmt"
	"strings"
)

// nodeTypeEmpty is an empty node.
const nodeTypeEmpty nodeType = 1

// nodeTypeBranch is a branch node.
const nodeTypeBranch nodeType = 2

// nodeTypeLeaf is a leaf node.
const nodeTypeLeaf nodeType = 3

// nodeTypeBoundary is the boundary between blocks.
const nodeTypeBoundary nodeType = 4

// GetEnumValue returns the value of the node Type
func (v nodeType) GetEnumValue() uint64 { return uint64(v) }

// SetEnumValue sets the value. SetEnumValue returns false if the value is invalid.
func (v *nodeType) SetEnumValue(id uint64) bool {
	u := nodeType(id)
	switch u {
	case nodeTypeEmpty, nodeTypeBranch, nodeTypeLeaf, nodeTypeBoundary:
		*v = u
		return true
	}
	return false
}

// String returns the name of the node Type.
func (v nodeType) String() string {
	switch v {
	case nodeTypeEmpty:
		return "empty"
	case nodeTypeBranch:
		return "branch"
	case nodeTypeLeaf:
		return "leaf"
	case nodeTypeBoundary:
		return "boundary"
	}
	return fmt.Sprintf("nodeType:%d", v)
}

// nodeTypeByName returns the named node Type.
func nodeTypeByName(name string) (nodeType, bool) {
	switch strings.ToLower(name) {
	case "empty":
		return nodeTypeEmpty, true
	case "branch":
		return nodeTypeBranch, true
	case "leaf":
		return nodeTypeLeaf, true
	case "boundary":
		return nodeTypeBoundary, true
	}
	return 0, false
}

// MarshalJSON marshals the node Type to JSON as a string.
func (v nodeType) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

// UnmarshalJSON unmarshals the node Type from JSON as a string.
func (v *nodeType) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	var ok bool
	*v, ok = nodeTypeByName(s)
	if !ok || strings.ContainsRune(v.String(), ':') {
		return fmt.Errorf("invalid node Type %q", s)
	}
	return nil
}
