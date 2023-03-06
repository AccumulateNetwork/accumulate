// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"time"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
)

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum --package api --out enums_gen.go enums.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package api types.yml responses.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-api --package api methods.yml

// TxFetchMode specifies how much detail of the transactions should be included in the result set
type TxFetchMode uint64

// BlockFilterMode specifies which blocks should be excluded
type BlockFilterMode uint64

type V3 interface {
	api.ConsensusService
	api.NetworkService
	api.MetricsService
	api.Querier
	api.Submitter
	api.Validator
	api.Faucet
	Private() private.Sequencer
}

type Options struct {
	Logger         log.Logger
	Describe       *config.Describe
	TxMaxWaitTime  time.Duration
	NetV3, LocalV3 V3
}
