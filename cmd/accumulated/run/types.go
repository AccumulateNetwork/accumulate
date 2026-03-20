// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
)

//go:generate go run gitlab.com/accumulatenetwork/core/schema/cmd/generate schema schema.yml -w schema_gen.go
//go:generate go run gitlab.com/accumulatenetwork/core/schema/cmd/generate types schema.yml -w types_gen.go
//go:generate go run github.com/rinchsan/gosimports/cmd/gosimports -w .

// TODO: Remove (once schema supports it)
type RouterServiceRef = ServiceOrRef[*RouterService]

type resetable interface {
	reset(inst *Instance) error
}

type prestarter interface {
	prestart(inst *Instance) error
}

type Configuration interface {
	Type() ConfigurationType
	apply(inst *Instance, cfg *Config) error
}

type Service interface {
	ioc.Factory
	Type() ServiceType
	start(inst *Instance) error
}

type Storage interface {
	Type() StorageType
	setPath(path string)
	open(*Instance) (keyvalue.Beginner, error)
}

type PrivateKey interface {
	Type() PrivateKeyType
	get(inst *Instance) (address.Address, error)
}
