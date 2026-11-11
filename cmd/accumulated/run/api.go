// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	v3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

var (
	querierWantsConsensus = ioc.Wants[v3.ConsensusService](func(q *Querier) string { return q.Partition })

	// The partition's join state, when there is one. A joining node must not
	// answer a BPT page or an account read carrying a receipt: those are what
	// another node's pull reads, and its store is the one the pull is filling
	// (#4297).
	querierWantsNodeState = ioc.Wants[nodestate.Serving](func(q *Querier) string { return q.Partition })
	querierProvides       = ioc.Provides[v3.Querier](func(q *Querier) string { return q.Partition })

	networkNeedsEvents  = ioc.Needs[*events.Bus](func(n *NetworkService) string { return n.Partition })
	networkNeedsStorage = ioc.Needs[keyvalue.Beginner](func(n *NetworkService) string { return n.Partition })
	networkProvides     = ioc.Provides[v3.NetworkService](func(n *NetworkService) string { return n.Partition })

	metricsNeedsConsensus = ioc.Needs[v3.ConsensusService](func(m *MetricsService) string { return m.Partition })
	metricsNeedsQuerier   = ioc.Needs[v3.Querier](func(m *MetricsService) string { return m.Partition })
	metricsProvides       = ioc.Provides[v3.MetricsService](func(m *MetricsService) string { return m.Partition })

	eventsNeedsStorage = ioc.Needs[keyvalue.Beginner](func(e *EventsService) string { return e.Partition })
	eventsNeedsEvents  = ioc.Needs[*events.Bus](func(e *EventsService) string { return e.Partition })
	eventsProvides     = ioc.Provides[v3.EventService](func(e *EventsService) string { return e.Partition })
)

func (q *Querier) Requires() []ioc.Requirement {
	desc := []ioc.Requirement{
		querierWantsConsensus.Requirement(q),
		querierWantsNodeState.Requirement(q),
	}
	desc = append(desc, q.Storage.Required(q.Partition)...)
	return desc
}

func (q *Querier) Provides() []ioc.Provided {
	return []ioc.Provided{
		querierProvides.Provided(q),
	}
}

func (q *Querier) start(inst *Instance) error {
	store, err := q.Storage.open(inst, q.Partition)
	if err != nil {
		return err
	}

	consensus, err := querierWantsConsensus.Get(inst.services, q)
	if err != nil {
		return err
	}

	nodeState, err := q.nodeState(inst, consensus)
	if err != nil {
		return err
	}

	impl := api.NewQuerier(api.QuerierParams{
		Logger:    logging.NewSlogLogger(inst.logger).With("module", "api"),
		Partition: q.Partition,
		// Deep: an API read reaches whatever the caller asks for, and
		// the store answers protocol reads from a window (BlockchainDB
		// spec 1.3).  The executor keeps the windowed database.
		Database:  inst.database(store, logging.NewSlogLogger(inst.logger)).Deep(),
		Consensus: consensus,
		NodeState: nodeState,
		// A read in a section this node has not loaded names the
		// partition's other query peers to ask instead (#4485).
		Redirect: func(ctx context.Context) []string {
			if inst.p2p == nil {
				return nil
			}
			var out []string
			for _, id := range inst.p2p.Providers(ctx, v3.ServiceTypeQuery.AddressFor(q.Partition), 16) {
				if id != inst.p2p.ID() {
					out = append(out, id.String())
				}
			}
			return out
		},
	})
	// Advertised to the network once this node is ACTIVE for the partition
	// (DIFFERENCES E30); provided in-process at once.
	registerRpcServiceWhenActive(inst, nodeState, impl.Type().AddressFor(q.Partition), message.Querier{Querier: impl})
	return querierProvides.Register(inst.services, q, impl)
}

// nodeState is the partition's join state. A partition with a consensus
// service has one, so a querier that cannot find it fails to start rather than
// serve what it has not verified; a querier with no consensus service is
// standalone, and answers for its store, explicitly (#4335).
func (q *Querier) nodeState(inst *Instance, consensus v3.ConsensusService) (nodestate.Serving, error) {
	serving, err := querierWantsNodeState.Get(inst.services, q)
	if err != nil {
		return nil, err
	}
	if serving != nil {
		return serving, nil
	}
	if consensus != nil {
		return nil, errors.InternalError.WithFormat("querier %s: a consensus service is registered but no node state is; refusing to start open", q.Partition)
	}
	inst.logger.Info("Querier has no consensus service and answers everything", "partition", q.Partition)
	return nodestate.Always{}, nil
}

func (n *NetworkService) Requires() []ioc.Requirement {
	return []ioc.Requirement{
		networkNeedsEvents.Requirement(n),
		networkNeedsStorage.Requirement(n),
	}
}

func (n *NetworkService) Provides() []ioc.Provided {
	return []ioc.Provided{
		networkProvides.Provided(n),
	}
}

func (n *NetworkService) start(inst *Instance) error {
	events, err := networkNeedsEvents.Get(inst.services, n)
	if err != nil {
		return err
	}

	store, err := networkNeedsStorage.Get(inst.services, n)
	if err != nil {
		return err
	}

	impl := api.NewNetworkService(api.NetworkServiceParams{
		Logger:    logging.NewSlogLogger(inst.logger).With("module", "api"),
		Partition: n.Partition,
		Database:  inst.database(store, logging.NewSlogLogger(inst.logger)).Deep(),
		EventBus:  events,
	})
	registerRpcService(inst, impl.Type().AddressFor(n.Partition), message.NetworkService{NetworkService: impl})
	return networkProvides.Register(inst.services, n, impl)
}

func (m *MetricsService) Requires() []ioc.Requirement {
	return []ioc.Requirement{
		metricsNeedsConsensus.Requirement(m),
		metricsNeedsQuerier.Requirement(m),
	}
}

func (m *MetricsService) Provides() []ioc.Provided {
	return []ioc.Provided{
		metricsProvides.Provided(m),
	}
}

func (m *MetricsService) start(inst *Instance) error {
	consensus, err := metricsNeedsConsensus.Get(inst.services, m)
	if err != nil {
		return err
	}

	querier, err := metricsNeedsQuerier.Get(inst.services, m)
	if err != nil {
		return err
	}

	impl := api.NewMetricsService(api.MetricsServiceParams{
		Logger:  logging.NewSlogLogger(inst.logger).With("module", "api"),
		Node:    consensus,
		Querier: querier,
	})
	registerRpcService(inst, impl.Type().AddressFor(m.Partition), message.MetricsService{MetricsService: impl})
	return metricsProvides.Register(inst.services, m, impl)
}

func (e *EventsService) Requires() []ioc.Requirement {
	return []ioc.Requirement{
		eventsNeedsStorage.Requirement(e),
		eventsNeedsEvents.Requirement(e),
	}
}

func (e *EventsService) Provides() []ioc.Provided {
	return []ioc.Provided{
		eventsProvides.Provided(e),
	}
}

func (e *EventsService) start(inst *Instance) error {
	store, err := eventsNeedsStorage.Get(inst.services, e)
	if err != nil {
		return err
	}

	events, err := eventsNeedsEvents.Get(inst.services, e)
	if err != nil {
		return err
	}

	impl := api.NewEventService(api.EventServiceParams{
		Logger:    logging.NewSlogLogger(inst.logger).With("module", "api"),
		Partition: e.Partition,
		Database:  inst.database(store, logging.NewSlogLogger(inst.logger)).Deep(),
		EventBus:  events,
	})
	registerRpcService(inst, impl.Type().AddressFor(e.Partition), message.EventService{EventService: impl})
	return eventsProvides.Register(inst.services, e, impl)
}
