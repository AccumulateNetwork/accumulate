// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"errors"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/exp/promise"
	"golang.org/x/exp/slog"
)

var errNone = errors.New("none")

func work(mu *sync.Mutex, wg *sync.WaitGroup, fn func()) {
	defer wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	fn()
}

func maybe[V any](fn func() (V, bool)) func() promise.Result[V] {
	return func() promise.Result[V] {
		v, ok := fn()
		if !ok {
			return promise.ErrorOf[V](errNone)
		}
		return promise.ValueOf(v)
	}
}

func try[V any](fn func() (V, error)) func() promise.Result[V] {
	return func() promise.Result[V] {
		v, err := fn()
		if err != nil {
			return promise.ErrorOf[V](err)
		}
		return promise.ValueOf(v)
	}
}

func catchAndLog[V any](ctx context.Context, p promise.Promise[V], message string, args ...any) promise.Promise[V] {
	return promise.Catch(p, func(err error) promise.Result[V] {
		args = append(args, "error", err)
		slog.DebugCtx(ctx, message, args...)
		return promise.ErrorOf[V](err)
	})
}

func definitely[U, V any](fn func(U) V) func(U) promise.Result[V] {
	return func(u U) promise.Result[V] {
		return promise.ValueOf(fn(u))
	}
}

func done[V any](fn func(V)) func(V) promise.Result[any] {
	return func(v V) promise.Result[any] {
		fn(v)
		return promise.Value[any]{}
	}
}

func waitFor[V any](wg *sync.WaitGroup, p promise.Promise[V]) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = p.Result().Get()
	}()
}
