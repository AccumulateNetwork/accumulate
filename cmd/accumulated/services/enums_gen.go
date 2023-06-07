// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package services

// GENERATED BY go run ./tools/cmd/gen-enum. DO NOT EDIT.

import (
	"encoding/json"
	"fmt"
	"strings"
)

// KeyStorageTypeInline .
const KeyStorageTypeInline KeyStorageType = 1

// KeyStorageTypeFile .
const KeyStorageTypeFile KeyStorageType = 2

// ServiceTypeFaucet .
const ServiceTypeFaucet ServiceType = 1

// GetEnumValue returns the value of the Key Storage Type
func (v KeyStorageType) GetEnumValue() uint64 { return uint64(v) }

// SetEnumValue sets the value. SetEnumValue returns false if the value is invalid.
func (v *KeyStorageType) SetEnumValue(id uint64) bool {
	u := KeyStorageType(id)
	switch u {
	case KeyStorageTypeInline, KeyStorageTypeFile:
		*v = u
		return true
	}
	return false
}

// String returns the name of the Key Storage Type.
func (v KeyStorageType) String() string {
	switch v {
	case KeyStorageTypeInline:
		return "inline"
	case KeyStorageTypeFile:
		return "file"
	}
	return fmt.Sprintf("KeyStorageType:%d", v)
}

// KeyStorageTypeByName returns the named Key Storage Type.
func KeyStorageTypeByName(name string) (KeyStorageType, bool) {
	switch strings.ToLower(name) {
	case "inline":
		return KeyStorageTypeInline, true
	case "file":
		return KeyStorageTypeFile, true
	}
	return 0, false
}

// MarshalJSON marshals the Key Storage Type to JSON as a string.
func (v KeyStorageType) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

// UnmarshalJSON unmarshals the Key Storage Type from JSON as a string.
func (v *KeyStorageType) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	var ok bool
	*v, ok = KeyStorageTypeByName(s)
	if !ok || strings.ContainsRune(v.String(), ':') {
		return fmt.Errorf("invalid Key Storage Type %q", s)
	}
	return nil
}

// GetEnumValue returns the value of the Service Type
func (v ServiceType) GetEnumValue() uint64 { return uint64(v) }

// SetEnumValue sets the value. SetEnumValue returns false if the value is invalid.
func (v *ServiceType) SetEnumValue(id uint64) bool {
	u := ServiceType(id)
	switch u {
	case ServiceTypeFaucet:
		*v = u
		return true
	}
	return false
}

// String returns the name of the Service Type.
func (v ServiceType) String() string {
	switch v {
	case ServiceTypeFaucet:
		return "faucet"
	}
	return fmt.Sprintf("ServiceType:%d", v)
}

// ServiceTypeByName returns the named Service Type.
func ServiceTypeByName(name string) (ServiceType, bool) {
	switch strings.ToLower(name) {
	case "faucet":
		return ServiceTypeFaucet, true
	}
	return 0, false
}

// MarshalJSON marshals the Service Type to JSON as a string.
func (v ServiceType) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

// UnmarshalJSON unmarshals the Service Type from JSON as a string.
func (v *ServiceType) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	var ok bool
	*v, ok = ServiceTypeByName(s)
	if !ok || strings.ContainsRune(v.String(), ':') {
		return fmt.Errorf("invalid Service Type %q", s)
	}
	return nil
}
