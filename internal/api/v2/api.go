// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"time"

	"github.com/cometbft/cometbft/libs/log"
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
}

type Options struct {
	Logger        log.Logger
	Describe      *config.Describe
	TxMaxWaitTime time.Duration
	LocalV3       V3
	Querier       api.Querier
	Submitter     api.Submitter
	Faucet        api.Faucet
	Validator     api.Validator
	Sequencer     private.Sequencer
}
