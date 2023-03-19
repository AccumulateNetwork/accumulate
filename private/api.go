// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package private

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
)

type NetworkServiceParams = api.NetworkServiceParams
type QuerierParams = api.QuerierParams
type MetricsServiceParams = api.MetricsServiceParams
type EventServiceParams = api.EventServiceParams
type SequencerParams = api.SequencerParams

func NewNetworkService(opts NetworkServiceParams) *api.NetworkService {
	return api.NewNetworkService(opts)
}

func NewQuerier(opts QuerierParams) *api.Querier {
	return api.NewQuerier(opts)
}

func NewMetricsService(opts MetricsServiceParams) *api.MetricsService {
	return api.NewMetricsService(opts)
}

func NewEventService(opts EventServiceParams) *api.EventService {
	return api.NewEventService(opts)
}

func NewSequencer(opts SequencerParams) *api.Sequencer {
	return api.NewSequencer(opts)
}
