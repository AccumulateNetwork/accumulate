// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"io/fs"
	"log/slog"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

func (s *SubnodeService) Requires() []ioc.Requirement { return nil }
func (s *SubnodeService) Provides() []ioc.Provided    { return nil }

func (s *SubnodeService) start(inst *Instance) error {
	sub := new(Instance)
	sub.rootDir = inst.path(s.Name)
	sub.id = s.Name
	// Share the parent's group so that Stop waits for the sub-node hosts to
	// release their ports
	sub.running = inst.running
	sub.context = inst.context
	sub.shutdown = inst.shutdown
	sub.services = ioc.Registry{}
	sub.logger = inst.logger.With("node", s.Name)
	sub.p2p = inst.p2p
	// Share halt controllers with parent so HTTP handler can access them
	sub.parentInstance = inst

	sub.config = &Config{
		Network: inst.config.Network,
		P2P: &P2P{
			Key: s.NodeKey,
		},
	}

	// A netsim writes each sub-node's own P2P configuration (key, listen
	// addresses, bootstrap peers). Give the sub-node its own libp2p host and
	// GossipSub router so that its DAG-BFT services have gossip and batch
	// fetch, with a peer ID of their own (#4392). The router is created
	// before any service connects the host (#4586).
	err := s.startP2P(inst, sub)
	if err != nil {
		return err
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
			slog.InfoContext(inst.context, "Starting", "subnode", s.Name, "service", svc.Type(), "module", "run")
			err := svc.start(sub)
			if err != nil {
				return errors.UnknownError.WithFormat("start service %v: %w", svc.Type(), err)
			}
		}
	}

	return nil
}

// startP2P starts the sub-node's own P2P node from the configuration written
// for it, if there is one. Otherwise the sub-node shares its parent's.
func (s *SubnodeService) startP2P(inst, sub *Instance) error {
	// Only a node that runs consensus needs a host of its own
	var consensus bool
	for _, svc := range s.Services {
		if _, ok := svc.(*DAGBFTService); ok {
			consensus = true
		}
	}
	if !consensus || !inst.netsimHasPeers() {
		return nil
	}

	file := inst.path(s.Name, "accumulate.toml")
	if _, err := os.Stat(file); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}

	cfg := new(Config)
	err := cfg.LoadFrom(file)
	if err != nil {
		return errors.UnknownError.WithFormat("load sub-node %s configuration: %w", s.Name, err)
	}
	if cfg.P2P == nil || len(cfg.P2P.Listen) == 0 {
		return nil
	}
	if cfg.P2P.Key == nil {
		cfg.P2P.Key = s.NodeKey
	}
	sub.config.P2P = cfg.P2P

	sub.p2p, sub.pubsub = nil, nil
	err = sub.config.P2P.start(sub)
	if err != nil {
		return errors.UnknownError.WithFormat("start p2p for %s: %w", s.Name, err)
	}
	sub.pubsub = sub.p2p.Pubsub()
	if sub.pubsub == nil {
		return errors.InternalError.With("p2p node has no pubsub router")
	}
	return nil
}

// netsimHasPeers returns true if the netsim has more than one consensus node,
// that is, more than one validator or follower per BVN, or more than one BVN
// (every node also joins the Directory, so two BVNs make a Directory of two).
// A lone node has no one to gossip with, and the API node that a client, or
// `init node`, is pointed at must be the node that serves its consensus
// services, so it keeps the root's host.
func (i *Instance) netsimHasPeers() bool {
	for _, c := range i.config.Configurations {
		if n, ok := c.(*NetSimConfiguration); ok {
			return n.Validators+n.Followers > 1 || n.Bvns > 1
		}
	}
	return false
}
