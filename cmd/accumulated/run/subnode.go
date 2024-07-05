// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

func (s *SubnodeService) Requires() []ioc.Requirement { return nil }
func (s *SubnodeService) Provides() []ioc.Provided    { return nil }

func (s *SubnodeService) start(inst *Instance) error {
	sub := new(Instance)
	sub.rootDir = inst.path(s.Name)
	sub.id = s.Name
	sub.running = new(sync.WaitGroup)
	sub.context = inst.context
	sub.shutdown = inst.shutdown
	sub.services = ioc.Registry{}
	sub.logger = inst.logger.With("node", s.Name)
	sub.p2p = inst.p2p

	sub.config = &Config{
		Network: inst.config.Network,
		P2P: &P2P{
			Key: s.NodeKey,
		},
	}

	// Determine initialization order
	services, err := ioc.Solve(s.Services)
	if err != nil {
		return err
	}

	// Prestart
	for _, services := range services {
		for _, svc := range services {
			svc, ok := svc.(prestarter)
			if !ok {
				continue
			}
			err = svc.prestart(sub)
			if err != nil {
				return errors.UnknownError.WithFormat("prestart service %T: %w", svc, err)
			}
		}
	}

	// Start services
	for _, services := range services {
		for _, svc := range services {
			inst.logger.InfoContext(inst.context, "Starting", "subnode", s.Name, "service", svc.Type(), "module", "run")
			err := svc.start(sub)
			if err != nil {
				return errors.UnknownError.WithFormat("start service %v: %w", svc.Type(), err)
			}
		}
	}

	return nil
}
