// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	v3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
)

var (
	querierWantsConsensus = ioc.Wants[v3.ConsensusService](func(q *Querier) string { return q.Partition })
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

	impl := api.NewQuerier(api.QuerierParams{
		Logger:    (*logging.Slogger)(inst.logger).With("module", "api"),
		Partition: q.Partition,
		Database:  database.New(store, (*logging.Slogger)(inst.logger)),
		Consensus: consensus,
	})
	registerRpcService(inst, impl.Type().AddressFor(q.Partition), message.Querier{Querier: impl})
	return querierProvides.Register(inst.services, q, impl)
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
		Logger:    (*logging.Slogger)(inst.logger).With("module", "api"),
		Partition: n.Partition,
		Database:  database.New(store, (*logging.Slogger)(inst.logger)),
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
		Logger:  (*logging.Slogger)(inst.logger).With("module", "api"),
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
		Logger:    (*logging.Slogger)(inst.logger).With("module", "api"),
		Partition: e.Partition,
		Database:  database.New(store, (*logging.Slogger)(inst.logger)),
		EventBus:  events,
	})
	registerRpcService(inst, impl.Type().AddressFor(e.Partition), message.EventService{EventService: impl})
	return eventsProvides.Register(inst.services, e, impl)
}

func registerRpcService(inst *Instance, addr *v3.ServiceAddress, service message.Service) {
	handler, err := message.NewHandler(service)
	if err != nil {
		panic(err)
	}
	inst.p2p.RegisterService(addr, handler.Handle)
}
