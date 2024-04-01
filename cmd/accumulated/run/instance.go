// Copyright 2024 The Accumulate Authors
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
	"time"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"golang.org/x/exp/slices"
	"golang.org/x/exp/slog"
)

type Instance struct {
	config  *Config
	rootDir string
	id      string

	running  *sync.WaitGroup    // tracks jobs that want a graceful shutdown
	context  context.Context    // canceled when the instance shuts down
	shutdown context.CancelFunc // shuts down the instance
	logger   *slog.Logger
	p2p      *p2p.Node
	services ioc.Registry
}

const minDiskSpace = 0.05

func Start(ctx context.Context, cfg *Config) (*Instance, error) {
	inst, err := New(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return inst, inst.Start()
}

func New(ctx context.Context, cfg *Config) (*Instance, error) {
	inst := new(Instance)
	inst.config = cfg
	inst.running = new(sync.WaitGroup)
	inst.context, inst.shutdown = context.WithCancel(ctx)
	inst.services = ioc.Registry{}

	var err error
	if cfg.file != "" {
		inst.rootDir, err = filepath.Abs(filepath.Dir(cfg.file))
	} else {
		inst.rootDir, err = os.Getwd()
	}
	if err != nil {
		return nil, err
	}

	// Setup logging
	err = cfg.Logging.start(inst)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("start logging: %w", err)
	}

	return inst, nil
}

func (inst *Instance) Reset() error {
	for _, c := range inst.config.Configurations {
		c, ok := c.(resetable)
		if !ok {
			continue
		}
		err := c.reset(inst)
		if err != nil {
			return errors.UnknownError.WithFormat("reset %T: %w", c, err)
		}
	}

	for _, s := range inst.services {
		s, ok := s.(resetable)
		if !ok {
			continue
		}
		err := s.reset(inst)
		if err != nil {
			return errors.UnknownError.WithFormat("reset %T: %w", s, err)
		}
	}
	return nil
}

func (inst *Instance) Start() error {
	return inst.StartFiltered(func(s Service) bool { return true })
}

func (inst *Instance) StartFiltered(predicate func(Service) bool) (err error) {
	// Cleanup if boot fails
	defer func() {
		if err != nil {
			inst.shutdown()
		}
	}()

	// Start metrics
	err = inst.config.Instrumentation.start(inst)
	if err != nil {
		return err
	}

	// Ensure the disk does not fill up (and is not currently full; requires
	// logging)
	free, err := diskUsage(inst.rootDir)
	if err != nil {
		return err
	} else if free < minDiskSpace {
		return errors.FatalError.With("disk is full")
	}
	go inst.checkDiskSpace()

	// Apply configurations
	for _, c := range inst.config.Configurations {
		err = c.apply(inst, inst.config)
		if err != nil {
			return err
		}
	}

	// Filter
	allServices := inst.config.Services
	if predicate != nil {
		allServices = slices.DeleteFunc(allServices, func(s Service) bool { return !predicate(s) })
	}

	// Determine initialization order
	services, err := ioc.Solve(allServices)
	if err != nil {
		return err
	}

	// Start the P2P node
	err = inst.config.P2P.start(inst)
	if err != nil {
		return errors.UnknownError.WithFormat("start p2p: %w", err)
	}

	// Prestart
	for _, services := range services {
		for _, svc := range services {
			svc, ok := svc.(prestarter)
			if !ok {
				continue
			}
			err = svc.prestart(inst)
			if err != nil {
				return errors.UnknownError.WithFormat("prestart service %T: %w", svc, err)
			}
		}
	}

	// Start services
	for _, services := range services {
		for _, svc := range services {
			inst.logger.InfoCtx(inst.context, "Starting", "module", "run", "service", svc.Type())
			err := svc.start(inst)
			if err != nil {
				return errors.UnknownError.WithFormat("start service %v: %w", svc.Type(), err)
			}
		}
	}

	return nil
}

func (i *Instance) Stop() {
	i.shutdown()
	i.running.Wait()
}

func (i *Instance) run(fn func()) {
	i.running.Add(1)
	go func() {
		defer i.running.Done()
		fn()
	}()
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

func (i *Instance) checkDiskSpace() {
	for {
		free, err := diskUsage(i.rootDir)
		if err != nil {
			i.logger.Error("Failed to get disk size, shutting down", "error", err, "module", "node")
			return
		}

		if free < 0.05 {
			i.logger.Error("Less than 5% disk space available, shutting down", "free", free, "module", "node")
			return
		}

		i.logger.Info("Disk usage", "free", free, "module", "node")

		time.Sleep(10 * time.Minute)
	}
}
