// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

// GENERATED BY go run ./tools/cmd/gen-enum. DO NOT EDIT.

import (
	"encoding/json"
	"fmt"
	"strings"
)

// PrivateKeyTypeRaw .
const PrivateKeyTypeRaw PrivateKeyType = 1

// PrivateKeyTypeTransient .
const PrivateKeyTypeTransient PrivateKeyType = 2

// PrivateKeyTypeSeed .
const PrivateKeyTypeSeed PrivateKeyType = 3

// PrivateKeyTypeCometPrivValFile .
const PrivateKeyTypeCometPrivValFile PrivateKeyType = 4

// PrivateKeyTypeCometNodeKeyFile .
const PrivateKeyTypeCometNodeKeyFile PrivateKeyType = 5

// GetEnumValue returns the value of the Private Key Type
func (v PrivateKeyType) GetEnumValue() uint64 { return uint64(v) }

// SetEnumValue sets the value. SetEnumValue returns false if the value is invalid.
func (v *PrivateKeyType) SetEnumValue(id uint64) bool {
	u := PrivateKeyType(id)
	switch u {
	case PrivateKeyTypeRaw, PrivateKeyTypeTransient, PrivateKeyTypeSeed, PrivateKeyTypeCometPrivValFile, PrivateKeyTypeCometNodeKeyFile:
		*v = u
		return true
	}
	return false
}

// String returns the name of the Private Key Type.
func (v PrivateKeyType) String() string {
	switch v {
	case PrivateKeyTypeRaw:
		return "raw"
	case PrivateKeyTypeTransient:
		return "transient"
	case PrivateKeyTypeSeed:
		return "seed"
	case PrivateKeyTypeCometPrivValFile:
		return "cometPrivValFile"
	case PrivateKeyTypeCometNodeKeyFile:
		return "cometNodeKeyFile"
	}
	return fmt.Sprintf("PrivateKeyType:%d", v)
}

// PrivateKeyTypeByName returns the named Private Key Type.
func PrivateKeyTypeByName(name string) (PrivateKeyType, bool) {
	switch strings.ToLower(name) {
	case "raw":
		return PrivateKeyTypeRaw, true
	case "transient":
		return PrivateKeyTypeTransient, true
	case "seed":
		return PrivateKeyTypeSeed, true
	case "cometprivvalfile":
		return PrivateKeyTypeCometPrivValFile, true
	case "cometnodekeyfile":
		return PrivateKeyTypeCometNodeKeyFile, true
	}
	return 0, false
}

// MarshalJSON marshals the Private Key Type to JSON as a string.
func (v PrivateKeyType) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

// UnmarshalJSON unmarshals the Private Key Type from JSON as a string.
func (v *PrivateKeyType) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	var ok bool
	*v, ok = PrivateKeyTypeByName(s)
	if !ok || strings.ContainsRune(v.String(), ':') {
		return fmt.Errorf("invalid Private Key Type %q", s)
	}
	return nil
}
