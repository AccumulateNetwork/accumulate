// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os/user"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/exp/promise"
)

// Variables needed by network.go
var (
	outputJSON  bool
	currentUser *user.User
)

func init() {
	var err error
	currentUser, err = user.Current()
	if err != nil {
		currentUser = &user.User{
			Username: "unknown",
			HomeDir:  "/tmp",
		}
	}
}

// work is a helper function for concurrent work
func work(mu *sync.Mutex, wg *sync.WaitGroup, fn func()) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		fn()
	}()
}

// maybe wraps a function that returns (T, bool) into a Result[T]
func maybe[T any](fn func() (T, bool)) func() promise.Result[T] {
	return func() promise.Result[T] {
		if v, ok := fn(); ok {
			return promise.ValueOf(v)
		}
		return promise.ErrorOf[T](fmt.Errorf("operation failed"))
	}
}

// waitFor waits for a promise to complete
func waitFor[T any](wg *sync.WaitGroup, promise promise.Promise[T]) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		promise.Result()
	}()
}

// done wraps a function to return a Result[any]
func done[T any](fn func(T)) func(T) promise.Result[any] {
	return func(v T) promise.Result[any] {
		fn(v)
		return promise.ValueOf[any](nil)
	}
}

// definitely wraps a function to always return a value
func definitely[T, S any](fn func(T) S) func(T) promise.Result[S] {
	return func(v T) promise.Result[S] {
		return promise.ValueOf(fn(v))
	}
}

// try wraps a function that returns (T, error) into a Result[T]
func try[T any](fn func() (T, error)) func() promise.Result[T] {
	return func() promise.Result[T] {
		if v, err := fn(); err == nil {
			return promise.ValueOf(v)
		} else {
			return promise.ErrorOf[T](err)
		}
	}
}

// catchAndLog logs errors from a promise
func catchAndLog[T any](ctx context.Context, promise promise.Promise[T], msg string, args ...any) {
	go func() {
		if _, err := promise.Result().Get(); err != nil {
			slog.ErrorContext(ctx, msg, append([]any{"error", err}, args...)...)
		}
	}()
}