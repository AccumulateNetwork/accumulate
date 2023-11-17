// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"golang.org/x/exp/slog"
)

type Instance struct {
	running *sync.WaitGroup
	context context.Context
	cancel  context.CancelFunc
	logger  *slog.Logger
	p2p     *p2p.Node
}

type nameAndType struct {
}

func Start(ctx context.Context, cfg *Config) (*Instance, error) {
	inst := new(Instance)
	inst.running = new(sync.WaitGroup)
	inst.context, inst.cancel = context.WithCancel(ctx)

	err := cfg.Logging.start(inst)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("start logging: %w", err)
	}

	err = cfg.P2P.start(inst)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("start p2p: %w", err)
	}

	return inst, nil
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
