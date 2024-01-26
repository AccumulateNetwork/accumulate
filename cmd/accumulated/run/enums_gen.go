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

// ServiceTypeStorage .
const ServiceTypeStorage ServiceType = 1

// ServiceTypeHttp .
const ServiceTypeHttp ServiceType = 7

// ServiceTypeRouter .
const ServiceTypeRouter ServiceType = 8

// StorageTypeMemory .
const StorageTypeMemory StorageType = 1

// StorageTypeBadger .
const StorageTypeBadger StorageType = 2

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

// GetEnumValue returns the value of the Service Type
func (v ServiceType) GetEnumValue() uint64 { return uint64(v) }

// SetEnumValue sets the value. SetEnumValue returns false if the value is invalid.
func (v *ServiceType) SetEnumValue(id uint64) bool {
	u := ServiceType(id)
	switch u {
	case ServiceTypeStorage, ServiceTypeHttp, ServiceTypeRouter:
		*v = u
		return true
	}
	return false
}

// String returns the name of the Service Type.
func (v ServiceType) String() string {
	switch v {
	case ServiceTypeStorage:
		return "storage"
	case ServiceTypeHttp:
		return "http"
	case ServiceTypeRouter:
		return "router"
	}
	return fmt.Sprintf("ServiceType:%d", v)
}

// ServiceTypeByName returns the named Service Type.
func ServiceTypeByName(name string) (ServiceType, bool) {
	switch strings.ToLower(name) {
	case "storage":
		return ServiceTypeStorage, true
	case "http":
		return ServiceTypeHttp, true
	case "router":
		return ServiceTypeRouter, true
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

// GetEnumValue returns the value of the Storage Type
func (v StorageType) GetEnumValue() uint64 { return uint64(v) }

// SetEnumValue sets the value. SetEnumValue returns false if the value is invalid.
func (v *StorageType) SetEnumValue(id uint64) bool {
	u := StorageType(id)
	switch u {
	case StorageTypeMemory, StorageTypeBadger:
		*v = u
		return true
	}
	return false
}

// String returns the name of the Storage Type.
func (v StorageType) String() string {
	switch v {
	case StorageTypeMemory:
		return "memory"
	case StorageTypeBadger:
		return "badger"
	}
	return fmt.Sprintf("StorageType:%d", v)
}

// StorageTypeByName returns the named Storage Type.
func StorageTypeByName(name string) (StorageType, bool) {
	switch strings.ToLower(name) {
	case "memory":
		return StorageTypeMemory, true
	case "badger":
		return StorageTypeBadger, true
	}
	return 0, false
}

// MarshalJSON marshals the Storage Type to JSON as a string.
func (v StorageType) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

// UnmarshalJSON unmarshals the Storage Type from JSON as a string.
func (v *StorageType) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	var ok bool
	*v, ok = StorageTypeByName(s)
	if !ok || strings.ContainsRune(v.String(), ':') {
		return fmt.Errorf("invalid Storage Type %q", s)
	}
	return nil
}
