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
	services map[ServiceDescriptor]any
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

	err = cfg.Logging.start(inst)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("start logging: %w", err)
	}

	err = cfg.P2P.start(inst)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("start p2p: %w", err)
	}

	err = cfg.startServices(inst)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return inst, nil
}

func (c *Config) startServices(inst *Instance) error {
	got := map[ServiceDescriptor]bool{}
	haveNeeded := func(s Service) bool {
		for _, n := range s.needs() {
			if !got[n] {
				return false
			}
		}
		return true
	}

	// Determine the order of initialization
	unsatisfied := c.Services
	var satisfied [][]Service
	for len(unsatisfied) > 0 {
		var unsatisfied2, satisfied2 []Service
		for _, s := range unsatisfied {
			if !haveNeeded(s) {
				unsatisfied2 = append(unsatisfied2, s)
				continue
			}

			satisfied2 = append(satisfied2, s)
			for _, p := range s.provides() {
				got[p] = true
			}
		}
		if len(satisfied2) == 0 {
			return errors.FatalError.With("unresolvable service dependency loop")
		}
		satisfied = append(satisfied, satisfied2)
		unsatisfied = unsatisfied2
	}

	// Start services
	for _, services := range satisfied {
		for _, svc := range services {
			err := svc.start(inst)
			if err != nil {
				return errors.UnknownError.WithFormat("start service: %w", err)
			}
		}
	}
	return nil
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
