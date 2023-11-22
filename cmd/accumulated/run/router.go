// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"gitlab.com/accumulatenetwork/accumulate/exp/apiutil"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
)

var (
	routerWantsEvents = ioc.Wants[*events.Bus](func(r *RouterService) string { return r.Events })
	routerProvides    = ioc.Provides[routing.Router](func(r *RouterService) string { return r.Name })
)

func (r *RouterService) Requires() []ioc.Requirement {
	events := routerWantsEvents.Requirement(r)
	if r.Events != "" {
		events.Optional = false
	}
	return []ioc.Requirement{events}
}

func (r *RouterService) Provides() []ioc.Provided {
	return []ioc.Provided{
		routerProvides.Provided(r),
	}
}

func (r *RouterService) create(inst *Instance) (routing.Router, error) {
	events, err := routerWantsEvents.Get(inst.services, r)
	if err != nil {
		return nil, err
	}

	return apiutil.InitRouter(apiutil.RouterOptions{
		Context: inst.context,
		Node:    inst.p2p,
		Network: inst.network,
		Events:  events,
		Logger:  (*logging.Slogger)(inst.logger),
	})
}

func (r *RouterService) start(inst *Instance) error {
	router, err := r.create(inst)
	if err != nil {
		return err
	}

	return routerProvides.Register(inst.services, r, router)
}
