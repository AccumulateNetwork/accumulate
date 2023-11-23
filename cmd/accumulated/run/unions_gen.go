// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

// GENERATED BY go run ./tools/cmd/gen-types. DO NOT EDIT.

import (
	"encoding/json"
	"fmt"
)

// NewStorage creates a new Storage for the specified StorageType.
func NewStorage(typ StorageType) (Storage, error) {
	switch typ {
	case StorageTypeBadger:
		return new(BadgerStorage), nil
	case StorageTypeMemory:
		return new(MemoryStorage), nil
	}
	return nil, fmt.Errorf("unknown storage %v", typ)
}

// EqualStorage is used to compare the values of the union
func EqualStorage(a, b Storage) bool {
	if a == b {
		return true
	}
	switch a := a.(type) {
	case *BadgerStorage:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*BadgerStorage)
		return ok && a.Equal(b)
	case *MemoryStorage:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*MemoryStorage)
		return ok && a.Equal(b)
	}
	return false
}

// CopyStorage copies a Storage.
func CopyStorage(v Storage) Storage {
	switch v := v.(type) {
	case *BadgerStorage:
		return v.Copy()
	case *MemoryStorage:
		return v.Copy()
	default:
		return v.CopyAsInterface().(Storage)
	}
}

// UnmarshalStorageJson unmarshals a Storage.
func UnmarshalStorageJSON(data []byte) (Storage, error) {
	var typ *struct{ Type StorageType }
	err := json.Unmarshal(data, &typ)
	if err != nil {
		return nil, err
	}

	if typ == nil {
		return nil, nil
	}

	acnt, err := NewStorage(typ.Type)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, acnt)
	if err != nil {
		return nil, err
	}

	return acnt, nil
}

// NewPrivateKey creates a new PrivateKey for the specified PrivateKeyType.
func NewPrivateKey(typ PrivateKeyType) (PrivateKey, error) {
	switch typ {
	case PrivateKeyTypeCometNodeKeyFile:
		return new(CometNodeKeyFile), nil
	case PrivateKeyTypeCometPrivValFile:
		return new(CometPrivValFile), nil
	case PrivateKeyTypeSeed:
		return new(PrivateKeySeed), nil
	case PrivateKeyTypeTransient:
		return new(TransientPrivateKey), nil
	}
	return nil, fmt.Errorf("unknown private key %v", typ)
}

// EqualPrivateKey is used to compare the values of the union
func EqualPrivateKey(a, b PrivateKey) bool {
	if a == b {
		return true
	}
	switch a := a.(type) {
	case *CometNodeKeyFile:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*CometNodeKeyFile)
		return ok && a.Equal(b)
	case *CometPrivValFile:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*CometPrivValFile)
		return ok && a.Equal(b)
	case *PrivateKeySeed:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*PrivateKeySeed)
		return ok && a.Equal(b)
	case *TransientPrivateKey:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*TransientPrivateKey)
		return ok && a.Equal(b)
	}
	return false
}

// CopyPrivateKey copies a PrivateKey.
func CopyPrivateKey(v PrivateKey) PrivateKey {
	switch v := v.(type) {
	case *CometNodeKeyFile:
		return v.Copy()
	case *CometPrivValFile:
		return v.Copy()
	case *PrivateKeySeed:
		return v.Copy()
	case *TransientPrivateKey:
		return v.Copy()
	default:
		return v.CopyAsInterface().(PrivateKey)
	}
}

// UnmarshalPrivateKeyJson unmarshals a PrivateKey.
func UnmarshalPrivateKeyJSON(data []byte) (PrivateKey, error) {
	var typ *struct{ Type PrivateKeyType }
	err := json.Unmarshal(data, &typ)
	if err != nil {
		return nil, err
	}

	if typ == nil {
		return nil, nil
	}

	acnt, err := NewPrivateKey(typ.Type)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, acnt)
	if err != nil {
		return nil, err
	}

	return acnt, nil
}

// NewService creates a new Service for the specified ServiceType.
func NewService(typ ServiceType) (Service, error) {
	switch typ {
	case ServiceTypeConsensus:
		return new(ConsensusService), nil
	case ServiceTypeEvents:
		return new(EventsService), nil
	case ServiceTypeHttp:
		return new(HttpService), nil
	case ServiceTypeMetrics:
		return new(MetricsService), nil
	case ServiceTypeNetwork:
		return new(NetworkService), nil
	case ServiceTypeQuerier:
		return new(Querier), nil
	case ServiceTypeRouter:
		return new(RouterService), nil
	case ServiceTypeStorage:
		return new(StorageService), nil
	}
	return nil, fmt.Errorf("unknown service %v", typ)
}

// EqualService is used to compare the values of the union
func EqualService(a, b Service) bool {
	if a == b {
		return true
	}
	switch a := a.(type) {
	case *ConsensusService:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*ConsensusService)
		return ok && a.Equal(b)
	case *EventsService:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*EventsService)
		return ok && a.Equal(b)
	case *HttpService:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*HttpService)
		return ok && a.Equal(b)
	case *MetricsService:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*MetricsService)
		return ok && a.Equal(b)
	case *NetworkService:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*NetworkService)
		return ok && a.Equal(b)
	case *Querier:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*Querier)
		return ok && a.Equal(b)
	case *RouterService:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*RouterService)
		return ok && a.Equal(b)
	case *StorageService:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*StorageService)
		return ok && a.Equal(b)
	}
	return false
}

// CopyService copies a Service.
func CopyService(v Service) Service {
	switch v := v.(type) {
	case *ConsensusService:
		return v.Copy()
	case *EventsService:
		return v.Copy()
	case *HttpService:
		return v.Copy()
	case *MetricsService:
		return v.Copy()
	case *NetworkService:
		return v.Copy()
	case *Querier:
		return v.Copy()
	case *RouterService:
		return v.Copy()
	case *StorageService:
		return v.Copy()
	default:
		return v.CopyAsInterface().(Service)
	}
}

// UnmarshalServiceJson unmarshals a Service.
func UnmarshalServiceJSON(data []byte) (Service, error) {
	var typ *struct{ Type ServiceType }
	err := json.Unmarshal(data, &typ)
	if err != nil {
		return nil, err
	}

	if typ == nil {
		return nil, nil
	}

	acnt, err := NewService(typ.Type)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, acnt)
	if err != nil {
		return nil, err
	}

	return acnt, nil
}

// NewConsensusApp creates a new ConsensusApp for the specified ConsensusAppType.
func NewConsensusApp(typ ConsensusAppType) (ConsensusApp, error) {
	switch typ {
	case ConsensusAppTypeCore:
		return new(CoreConsensusApp), nil
	}
	return nil, fmt.Errorf("unknown consensus app %v", typ)
}

// EqualConsensusApp is used to compare the values of the union
func EqualConsensusApp(a, b ConsensusApp) bool {
	if a == b {
		return true
	}
	switch a := a.(type) {
	case *CoreConsensusApp:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*CoreConsensusApp)
		return ok && a.Equal(b)
	}
	return false
}

// CopyConsensusApp copies a ConsensusApp.
func CopyConsensusApp(v ConsensusApp) ConsensusApp {
	switch v := v.(type) {
	case *CoreConsensusApp:
		return v.Copy()
	default:
		return v.CopyAsInterface().(ConsensusApp)
	}
}

// UnmarshalConsensusAppJson unmarshals a ConsensusApp.
func UnmarshalConsensusAppJSON(data []byte) (ConsensusApp, error) {
	var typ *struct{ Type ConsensusAppType }
	err := json.Unmarshal(data, &typ)
	if err != nil {
		return nil, err
	}

	if typ == nil {
		return nil, nil
	}

	acnt, err := NewConsensusApp(typ.Type)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, acnt)
	if err != nil {
		return nil, err
	}

	return acnt, nil
}

// NewConfiguration creates a new Configuration for the specified ConfigurationType.
func NewConfiguration(typ ConfigurationType) (Configuration, error) {
	switch typ {
	case ConfigurationTypeCoreValidator:
		return new(CoreValidatorConfiguration), nil
	}
	return nil, fmt.Errorf("unknown configuration %v", typ)
}

// EqualConfiguration is used to compare the values of the union
func EqualConfiguration(a, b Configuration) bool {
	if a == b {
		return true
	}
	switch a := a.(type) {
	case *CoreValidatorConfiguration:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*CoreValidatorConfiguration)
		return ok && a.Equal(b)
	}
	return false
}

// CopyConfiguration copies a Configuration.
func CopyConfiguration(v Configuration) Configuration {
	switch v := v.(type) {
	case *CoreValidatorConfiguration:
		return v.Copy()
	default:
		return v.CopyAsInterface().(Configuration)
	}
}

// UnmarshalConfigurationJson unmarshals a Configuration.
func UnmarshalConfigurationJSON(data []byte) (Configuration, error) {
	var typ *struct{ Type ConfigurationType }
	err := json.Unmarshal(data, &typ)
	if err != nil {
		return nil, err
	}

	if typ == nil {
		return nil, nil
	}

	acnt, err := NewConfiguration(typ.Type)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, acnt)
	if err != nil {
		return nil, err
	}

	return acnt, nil
}
