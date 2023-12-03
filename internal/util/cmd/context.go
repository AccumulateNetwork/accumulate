// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package cmdutil

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
)

var ErrInterrupted = errors.New("interrupted")

// ContextForMainProcess creates a context that is canceled if the process
// receives SIGINT or SIGTERM. Once SIGINT/SIGTERM is received the handler is
// removed, so a second signal will terminate the process as normal. When
// canceled via a signal, [context.Context.Error] will return [ErrInterrupted].
func ContextForMainProcess(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancelCause(ctx)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		signal.Stop(sigs)
		cancel(ErrInterrupted)
	}()

	return ctx
}
