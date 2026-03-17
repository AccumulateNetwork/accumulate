// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build !dagbft

package run

import (
	types "github.com/cometbft/cometbft/abci/types"
	tmnode "github.com/cometbft/cometbft/node"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ConsensusApp defines the interface for consensus applications using CometBFT.
type ConsensusApp interface {
	Type() ConsensusAppType
	partition() *protocol.PartitionInfo
	Requires() []ioc.Requirement
	Provides() []ioc.Provided
	prestart(*Instance) error
	start(*Instance, *tendermint) (types.Application, error)
	register(*Instance, *tendermint, *tmnode.Node) error
}
