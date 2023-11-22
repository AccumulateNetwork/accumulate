// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	v3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
)

var (
	querierWantsConsensus = wants[v3.ConsensusService](func(q *Querier) string { return q.Partition })
	querierProvides       = provides[v3.Querier](func(q *Querier) string { return q.Partition })

	networkNeedsEvents  = needs[*events.Bus](func(n *NetworkService) string { return n.Partition })
	networkNeedsStorage = needs[keyvalue.Beginner](func(n *NetworkService) string { return n.Partition })
	networkProvides     = provides[v3.NetworkService](func(n *NetworkService) string { return n.Partition })

	metricsNeedsConsensus = needs[v3.ConsensusService](func(m *MetricsService) string { return m.Partition })
	metricsNeedsQuerier   = needs[v3.Querier](func(m *MetricsService) string { return m.Partition })
	metricsProvides       = provides[v3.MetricsService](func(m *MetricsService) string { return m.Partition })

	eventsNeedsStorage = needs[keyvalue.Beginner](func(e *EventsService) string { return e.Partition })
	eventsNeedsEvents  = needs[*events.Bus](func(e *EventsService) string { return e.Partition })
	eventsProvides     = provides[v3.EventService](func(e *EventsService) string { return e.Partition })
)

func (q *Querier) needs() []ServiceDescriptor {
	desc := []ServiceDescriptor{
		querierWantsConsensus.with(q),
	}
	desc = append(desc, q.Storage.needs(q.Partition)...)
	return desc
}

func (q *Querier) provides() []ServiceDescriptor {
	return []ServiceDescriptor{
		querierProvides.with(q),
	}
}

func (q *Querier) start(inst *Instance) error {
	store, err := q.Storage.open(inst, q.Partition)
	if err != nil {
		return err
	}

	consensus, err := querierWantsConsensus.get(inst, q)
	if err != nil {
		return err
	}

	impl := api.NewQuerier(api.QuerierParams{
		Logger:    (*logging.Slogger)(inst.logger).With("module", "api"),
		Partition: q.Partition,
		Database:  database.New(store, (*logging.Slogger)(inst.logger)),
		Consensus: consensus,
	})
	registerRpcService(inst, impl.Type().AddressFor(q.Partition), message.Querier{Querier: impl})
	return querierProvides.register(inst, q, impl)
}

func (n *NetworkService) needs() []ServiceDescriptor {
	return []ServiceDescriptor{
		networkNeedsEvents.with(n),
		networkNeedsStorage.with(n),
	}
}

func (n *NetworkService) provides() []ServiceDescriptor {
	return []ServiceDescriptor{
		networkProvides.with(n),
	}
}

func (n *NetworkService) start(inst *Instance) error {
	events, err := networkNeedsEvents.get(inst, n)
	if err != nil {
		return err
	}

	store, err := networkNeedsStorage.get(inst, n)
	if err != nil {
		return err
	}

	impl := api.NewNetworkService(api.NetworkServiceParams{
		Logger:    (*logging.Slogger)(inst.logger).With("module", "api"),
		Partition: n.Partition,
		Database:  database.New(store, (*logging.Slogger)(inst.logger)),
		EventBus:  events,
	})
	registerRpcService(inst, impl.Type().AddressFor(n.Partition), message.NetworkService{NetworkService: impl})
	return networkProvides.register(inst, n, impl)
}

func (m *MetricsService) needs() []ServiceDescriptor {
	return []ServiceDescriptor{
		metricsNeedsConsensus.with(m),
		metricsNeedsQuerier.with(m),
	}
}

func (m *MetricsService) provides() []ServiceDescriptor {
	return []ServiceDescriptor{
		metricsProvides.with(m),
	}
}

func (m *MetricsService) start(inst *Instance) error {
	consensus, err := metricsNeedsConsensus.get(inst, m)
	if err != nil {
		return err
	}

	querier, err := metricsNeedsQuerier.get(inst, m)
	if err != nil {
		return err
	}

	impl := api.NewMetricsService(api.MetricsServiceParams{
		Logger:  (*logging.Slogger)(inst.logger).With("module", "api"),
		Node:    consensus,
		Querier: querier,
	})
	registerRpcService(inst, impl.Type().AddressFor(m.Partition), message.MetricsService{MetricsService: impl})
	return metricsProvides.register(inst, m, impl)
}

func (e *EventsService) needs() []ServiceDescriptor {
	return []ServiceDescriptor{
		eventsNeedsStorage.with(e),
		eventsNeedsEvents.with(e),
	}
}

func (e *EventsService) provides() []ServiceDescriptor {
	return []ServiceDescriptor{
		eventsProvides.with(e),
	}
}

func (e *EventsService) start(inst *Instance) error {
	store, err := eventsNeedsStorage.get(inst, e)
	if err != nil {
		return err
	}

	events, err := eventsNeedsEvents.get(inst, e)
	if err != nil {
		return err
	}

	impl := api.NewEventService(api.EventServiceParams{
		Logger:    (*logging.Slogger)(inst.logger).With("module", "api"),
		Partition: e.Partition,
		Database:  database.New(store, (*logging.Slogger)(inst.logger)),
		EventBus:  events,
	})
	registerRpcService(inst, impl.Type().AddressFor(e.Partition), message.EventService{EventService: impl})
	return eventsProvides.register(inst, e, impl)
}

func registerRpcService(inst *Instance, addr *v3.ServiceAddress, service message.Service) {
	handler, err := message.NewHandler(service)
	if err != nil {
		panic(err)
	}
	inst.p2p.RegisterService(addr, handler.Handle)
}
