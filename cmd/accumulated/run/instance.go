// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"golang.org/x/exp/slog"
)

type Instance struct {
	network string
	rootDir string

	running  *sync.WaitGroup
	context  context.Context
	cancel   context.CancelFunc
	logger   *slog.Logger
	p2p      *p2p.Node
	services map[serviceKey]any
}

type serviceKey struct {
	Name string
	Type reflect.Type
}

type nameAndType struct {
}

func Start(ctx context.Context, cfg *Config) (_ *Instance, err error) {
	inst := new(Instance)
	inst.running = new(sync.WaitGroup)
	inst.context, inst.cancel = context.WithCancel(ctx)

	defer func() {
		if err != nil {
			inst.cancel()
		}
	}()

	if cfg.file != "" {
		inst.rootDir, err = filepath.Abs(filepath.Dir(cfg.file))
	} else {
		inst.rootDir, err = os.Getwd()
	}
	if err != nil {
		return nil, err
	}

	services, err := cfg.orderServices()
	if err != nil {
		return nil, err
	}

	err = cfg.Logging.start(inst)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("start logging: %w", err)
	}

	err = cfg.P2P.start(inst)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("start p2p: %w", err)
	}

	// Start services
	for _, services := range services {
		for _, svc := range services {
			err := svc.start(inst)
			if err != nil {
				return nil, errors.UnknownError.WithFormat("start service %T: %w", svc, err)
			}
		}
	}

	return inst, nil
}

func desc2key(d ServiceDescriptor) serviceKey {
	return serviceKey{
		Name: strings.ToLower(d.Name()),
		Type: d.Type(),
	}
}

func (c *Config) orderServices() ([][]Service, error) {
	got := map[serviceKey]bool{}
	haveNeeded := func(s Service) bool {
		for _, n := range s.needs() {
			if !n.Optional() && !got[desc2key(n)] {
				return false
			}
		}
		return true
	}
	haveWanted := func(s Service) bool {
		for _, n := range s.needs() {
			if n.Optional() && !got[desc2key(n)] {
				return false
			}
		}
		return true
	}

	// Determine the order of initialization
	unsatisfied := make([]Service, 0, len(c.Apps)+len(c.Services))
	unsatisfied = append(unsatisfied, c.Apps...)
	unsatisfied = append(unsatisfied, c.Services...)
	var satisfied [][]Service
	for len(unsatisfied) > 0 {
		var unsatisfied2, satisfied2, maybe []Service
		for _, s := range unsatisfied {
			switch {
			case !haveNeeded(s):
				unsatisfied2 = append(unsatisfied2, s)
			case !haveWanted(s):
				maybe = append(maybe, s)
			default:
				satisfied2 = append(satisfied2, s)
			}
		}

		switch {
		case len(satisfied2) > 0:
			unsatisfied2 = append(unsatisfied2, maybe...)
		case len(maybe) > 0:
			satisfied2 = maybe
		default:
			return nil, errors.FatalError.With("unresolvable service dependency loop")
		}

		for _, s := range satisfied2 {
			for _, p := range s.provides() {
				got[desc2key(p)] = true
			}
		}

		satisfied = append(satisfied, satisfied2)
		unsatisfied = unsatisfied2
	}

	return satisfied, nil
}

func (i *Instance) Stop() error {
	i.cancel()
	i.running.Wait()
	return nil
}

func (i *Instance) cleanup(fn func()) {
	i.running.Add(1)
	go func() {
		defer i.running.Done()
		<-i.context.Done()
		fn()
	}()
}

func (i *Instance) path(path ...string) string {
	if len(path) == 0 {
		return i.rootDir
	}
	if filepath.IsAbs(path[0]) {
		return filepath.Join(path...)
	}
	return filepath.Join(append([]string{i.rootDir}, path...)...)
}
