// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"golang.org/x/exp/slog"
)

type Instance struct {
	network string
	rootDir string

	running  *sync.WaitGroup    // tracks jobs that want a graceful shutdown
	context  context.Context    // canceled when the instance shuts down
	shutdown context.CancelFunc // shuts down the instance
	logger   *slog.Logger
	p2p      *p2p.Node
	services ioc.Registry
}

const minDiskSpace = 0.05

func Start(ctx context.Context, cfg *Config) (_ *Instance, err error) {
	inst := new(Instance)
	inst.network = cfg.Network
	inst.running = new(sync.WaitGroup)
	inst.context, inst.shutdown = context.WithCancel(ctx)
	inst.services = ioc.Registry{}

	defer func() {
		if err != nil {
			inst.shutdown()
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

	// Determine initialization order
	services, err := ioc.Solve(cfg.Services)
	if err != nil {
		return nil, err
	}

	// Setup logging
	err = cfg.Logging.start(inst)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("start logging: %w", err)
	}

	// Ensure the disk does not fill up (and is not currently full; requires
	// logging)
	free, err := diskUsage(inst.rootDir)
	if err != nil {
		return nil, err
	} else if free < minDiskSpace {
		return nil, errors.FatalError.With("disk is full")
	}
	go inst.checkDiskSpace()

	// Start the P2P node
	err = cfg.P2P.start(inst)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("start p2p: %w", err)
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
				return nil, errors.UnknownError.WithFormat("prestart service %T: %w", svc, err)
			}
		}
	}

	// Start services
	for _, services := range services {
		for _, svc := range services {
			slog.InfoCtx(inst.context, "Starting", "service", fmt.Sprintf("%T", svc))
			err := svc.start(inst)
			if err != nil {
				return nil, errors.UnknownError.WithFormat("start service %T: %w", svc, err)
			}
		}
	}

	return inst, nil
}

func (i *Instance) Stop() error {
	i.shutdown()
	i.running.Wait()
	return nil
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
