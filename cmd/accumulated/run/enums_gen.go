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

// ConfigurationTypeCoreValidator .
const ConfigurationTypeCoreValidator ConfigurationType = 1

// ConfigurationTypeGateway .
const ConfigurationTypeGateway ConfigurationType = 2

// ConsensusAppTypeCore .
const ConsensusAppTypeCore ConsensusAppType = 1

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

// ServiceTypeConsensus .
const ServiceTypeConsensus ServiceType = 2

// ServiceTypeQuerier .
const ServiceTypeQuerier ServiceType = 3

// ServiceTypeNetwork .
const ServiceTypeNetwork ServiceType = 4

// ServiceTypeMetrics .
const ServiceTypeMetrics ServiceType = 5

// ServiceTypeEvents .
const ServiceTypeEvents ServiceType = 6

// ServiceTypeHttp .
const ServiceTypeHttp ServiceType = 7

// ServiceTypeRouter .
const ServiceTypeRouter ServiceType = 8

// ServiceTypeSnapshot .
const ServiceTypeSnapshot ServiceType = 9

// ServiceTypeFaucet .
const ServiceTypeFaucet ServiceType = 10

// StorageTypeMemory .
const StorageTypeMemory StorageType = 1

// StorageTypeBadger .
const StorageTypeBadger StorageType = 2

// GetEnumValue returns the value of the Configuration Type
func (v ConfigurationType) GetEnumValue() uint64 { return uint64(v) }

// SetEnumValue sets the value. SetEnumValue returns false if the value is invalid.
func (v *ConfigurationType) SetEnumValue(id uint64) bool {
	u := ConfigurationType(id)
	switch u {
	case ConfigurationTypeCoreValidator, ConfigurationTypeGateway:
		*v = u
		return true
	}
	return false
}

// String returns the name of the Configuration Type.
func (v ConfigurationType) String() string {
	switch v {
	case ConfigurationTypeCoreValidator:
		return "coreValidator"
	case ConfigurationTypeGateway:
		return "gateway"
	}
	return fmt.Sprintf("ConfigurationType:%d", v)
}

// ConfigurationTypeByName returns the named Configuration Type.
func ConfigurationTypeByName(name string) (ConfigurationType, bool) {
	switch strings.ToLower(name) {
	case "corevalidator":
		return ConfigurationTypeCoreValidator, true
	case "gateway":
		return ConfigurationTypeGateway, true
	}
	return 0, false
}

// MarshalJSON marshals the Configuration Type to JSON as a string.
func (v ConfigurationType) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

// UnmarshalJSON unmarshals the Configuration Type from JSON as a string.
func (v *ConfigurationType) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	var ok bool
	*v, ok = ConfigurationTypeByName(s)
	if !ok || strings.ContainsRune(v.String(), ':') {
		return fmt.Errorf("invalid Configuration Type %q", s)
	}
	return nil
}

// GetEnumValue returns the value of the Consensus App Type
func (v ConsensusAppType) GetEnumValue() uint64 { return uint64(v) }

// SetEnumValue sets the value. SetEnumValue returns false if the value is invalid.
func (v *ConsensusAppType) SetEnumValue(id uint64) bool {
	u := ConsensusAppType(id)
	switch u {
	case ConsensusAppTypeCore:
		*v = u
		return true
	}
	return false
}

// String returns the name of the Consensus App Type.
func (v ConsensusAppType) String() string {
	switch v {
	case ConsensusAppTypeCore:
		return "core"
	}
	return fmt.Sprintf("ConsensusAppType:%d", v)
}

// ConsensusAppTypeByName returns the named Consensus App Type.
func ConsensusAppTypeByName(name string) (ConsensusAppType, bool) {
	switch strings.ToLower(name) {
	case "core":
		return ConsensusAppTypeCore, true
	}
	return 0, false
}

// MarshalJSON marshals the Consensus App Type to JSON as a string.
func (v ConsensusAppType) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

// UnmarshalJSON unmarshals the Consensus App Type from JSON as a string.
func (v *ConsensusAppType) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	var ok bool
	*v, ok = ConsensusAppTypeByName(s)
	if !ok || strings.ContainsRune(v.String(), ':') {
		return fmt.Errorf("invalid Consensus App Type %q", s)
	}
	return nil
}

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
	case ServiceTypeStorage, ServiceTypeConsensus, ServiceTypeQuerier, ServiceTypeNetwork, ServiceTypeMetrics, ServiceTypeEvents, ServiceTypeHttp, ServiceTypeRouter, ServiceTypeSnapshot, ServiceTypeFaucet:
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
	case ServiceTypeConsensus:
		return "consensus"
	case ServiceTypeQuerier:
		return "querier"
	case ServiceTypeNetwork:
		return "network"
	case ServiceTypeMetrics:
		return "metrics"
	case ServiceTypeEvents:
		return "events"
	case ServiceTypeHttp:
		return "http"
	case ServiceTypeRouter:
		return "router"
	case ServiceTypeSnapshot:
		return "snapshot"
	case ServiceTypeFaucet:
		return "faucet"
	}
	return fmt.Sprintf("ServiceType:%d", v)
}

// ServiceTypeByName returns the named Service Type.
func ServiceTypeByName(name string) (ServiceType, bool) {
	switch strings.ToLower(name) {
	case "storage":
		return ServiceTypeStorage, true
	case "consensus":
		return ServiceTypeConsensus, true
	case "querier":
		return ServiceTypeQuerier, true
	case "network":
		return ServiceTypeNetwork, true
	case "metrics":
		return ServiceTypeMetrics, true
	case "events":
		return ServiceTypeEvents, true
	case "http":
		return ServiceTypeHttp, true
	case "router":
		return ServiceTypeRouter, true
	case "snapshot":
		return ServiceTypeSnapshot, true
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
