// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	types "github.com/cometbft/cometbft/abci/types"
	tmnode "github.com/cometbft/cometbft/node"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
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

type ConsensusApp interface {
	Type() ConsensusAppType
	partition() *protocol.PartitionInfo
	Requires() []ioc.Requirement
	Provides() []ioc.Provided
	prestart(*Instance) error
	start(*Instance, *tendermint) (types.Application, error)
	register(*Instance, *tendermint, *tmnode.Node) error
}
