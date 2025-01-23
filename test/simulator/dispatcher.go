// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// fakeDispatcher drops everything
type fakeDispatcher struct{}

func (fakeDispatcher) Close() { /* Nothing to do */ }

func (fakeDispatcher) Submit(ctx context.Context, dest *url.URL, envelope *messaging.Envelope) error {
	// Drop it
	return nil
}

func (fakeDispatcher) Send(context.Context) <-chan error {
	// Nothing to do
	ch := make(chan error)
	close(ch)
	return ch
}

type closeDispatcher struct {
	execute.Dispatcher
	close func()
}

func (d *closeDispatcher) Close() {
	d.close()
	d.Dispatcher.Close()
}

type DispatchInterceptor = func(ctx context.Context, env *messaging.Envelope) (send bool, err error)

type interceptDispatcher struct {
	execute.Dispatcher
	interceptor DispatchInterceptor
}

func (d *interceptDispatcher) Submit(ctx context.Context, u *url.URL, env *messaging.Envelope) error {
	keep, err := d.interceptor(ctx, env)
	if !keep || err != nil {
		return err
	}

	return d.Dispatcher.Submit(ctx, u, env)
}
