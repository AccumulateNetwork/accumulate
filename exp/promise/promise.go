// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package promise

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"

	"golang.org/x/exp/slog"
)

// A Promise represents an asynchronous process.
type Promise[T any] interface {
	// Result returns the result of the promise.
	Result() Result[T]
}

// staticPromise satisfies Promise with a pre-computed result.
type staticPromise[T any] struct {
	result Result[T]
}

func (s staticPromise[T]) Result() Result[T] { return s.result }

// chanPromise retrieves the result of an asynchronous process via a channel.
type chanPromise[T any] struct {
	once   sync.Once
	ch     <-chan Result[T]
	result Result[T]
}

func (p *chanPromise[T]) Result() Result[T] {
	p.once.Do(func() {
		p.result = <-p.ch
	})
	return p.result
}

// ResultPromise is a [Promise] that satisfies [Result].
type ResultPromise[T any] struct{ Promise Promise[T] }

// Get returns the result of the promise.
func (v ResultPromise[T]) Get() (T, error) {
	return v.Promise.Result().Get()
}

// Result is the result of a promise.
type Result[T any] interface {
	// Get returns the resultant value or an error.
	Get() (T, error)
}

// Value is the [Result] of a successful process.
type Value[T any] struct{ Value T }

// Get returns the value.
func (v Value[T]) Get() (T, error) {
	return v.Value, nil
}

// Error is the [Result] of a failed process.
type Error[T any] struct{ Err error }

// Get returns the error.
func (e Error[T]) Get() (T, error) {
	var z T
	return z, e.Err
}

// Panic is the [Result] of a process that panicked.
type Panic[T any] struct{ Value any }

// Get returns an error with the panic details.
func (p Panic[T]) Get() (T, error) {
	var z T
	return z, fmt.Errorf("panicked: %v", p.Value)
}

// noResult is returned when a promise callback returns nil.
type noResult[T any] struct{}

func (noResult[T]) Get() (T, error) {
	var z T
	return z, errors.New("no result")
}

// Resolved returns a promise that resolves to a [Value].
func Resolved[T any](v T) Promise[T] {
	return staticPromise[T]{Value[T]{v}}
}

// Rejected returns a promise that resolves to an [Error].
func Rejected[T any](err error) Promise[T] {
	return staticPromise[T]{Error[T]{err}}
}

// ValueOf returns a [Value] for the given value.
func ValueOf[T any](v T) Value[T] { return Value[T]{v} }

// ErrorOf returns an [Error] for the given error and result type.
func ErrorOf[T any](err error) Error[T] { return Error[T]{err} }

// ValueOrError returns a [Value] if the error is nil, otherwise returning an
// [Error].
func ValueOrError[T any](v T, err error) Result[T] {
	if err != nil {
		return ErrorOf[T](err)
	}
	return ValueOf(v)
}

// The location of a package function.
var (
	pcThen     atomic.Uintptr
	pcCatch    atomic.Uintptr
	pcSyncCall atomic.Uintptr
	pcSyncThen atomic.Uintptr
)

// isMyFunc returns true if the program counter represents function in this
// package.
func isMyFunc(pc uintptr) bool {
	return false ||
		pcThen.Load() == pc ||
		pcCatch.Load() == pc ||
		pcSyncCall.Load() == pc ||
		pcSyncThen.Load() == pc
}

// slogNilResult logs a nil result error, including the caller function name and
// location when possible.
func slogNilResult(callers []uintptr) {
	for _, pc := range callers {
		if isMyFunc(pc) {
			continue
		}

		fn := runtime.FuncForPC(pc)
		if fn == nil {
			continue
		}

		file, line := fn.FileLine(pc)
		slog.Error("nil promise result", "func", fn.Name(), "file", file, "line", line)
		break
	}
	slog.Error("nil promise result", "func", "unknown")
}

// New returns a Promise that behaves more like traditional promises, returning
// resolve and reject callbacks.
func New[T any]() (_ Promise[T], resolve func(T), reject func(error)) {
	// Allocate a channel for the result and a [sync.Once] to avoid races.
	ch := make(chan Result[T], 1)
	var once sync.Once

	return &chanPromise[T]{ch: ch},
		func(v T) { once.Do(func() { ch <- Value[T]{v} }) },
		func(err error) { once.Do(func() { ch <- Error[T]{err} }) }
}

// Call spawns a concurrent process that calls the function.
func Call[T any](fn func() Result[T]) Promise[T] {
	// Get the caller's PC now, since doing so within the goroutine will not get
	// the result we want. Get two PCs since we may need to skip one (if the
	// immediate caller is another function in this package).
	var callers [2]uintptr
	numCallers := runtime.Callers(2, callers[:])

	// Allocate a channel for the result and a [sync.Once] to avoid races.
	ch := make(chan Result[T])
	var once sync.Once

	// Launch the process
	go func() {
		// Close the result channel once we're done
		defer close(ch)

		// Handle panics
		defer func() {
			if r := recover(); r != nil {
				once.Do(func() { ch <- Panic[T]{r} })
			}
		}()

		// Call the function
		r := fn()

		// Handle a nil result
		if r == nil {
			r = noResult[T]{}
			slogNilResult(callers[:numCallers])
		}

		// Send the result
		once.Do(func() { ch <- r })
	}()

	return &chanPromise[T]{ch: ch}
}

// Then spawns a concurrent process that waits for the promise to resolve and
// calls the function if the promise succeeds, passing the result to the
// function.
func Then[T, S any](promise Promise[T], fn func(T) Result[S]) Promise[S] {
	getPC(&pcThen)
	return Call(func() Result[S] {
		v, err := promise.Result().Get()
		if err != nil {
			return Error[S]{err}
		}
		return fn(v)
	})
}

// Catch spawns a concurrent process that waits for the promise to resolve and
// calls the function if the promise fails, passing the result to the function.
func Catch[T any](promise Promise[T], fn func(error) Result[T]) Promise[T] {
	getPC(&pcCatch)
	return Call(func() Result[T] {
		v, err := promise.Result().Get()
		if err == nil {
			return Value[T]{v}
		}
		return fn(err)
	})
}

// SyncCall spawns a process that acquires the lock, calls the function, and
// releases the lock.
func SyncCall[T any](mu sync.Locker, fn func() Result[T]) Promise[T] {
	getPC(&pcSyncCall)
	return Call(func() Result[T] {
		mu.Lock()
		defer mu.Unlock()
		return fn()
	})
}

// SyncThen spawns a process that waits for the promise to resolve; if the
// promise is successful, the process acquires the lock, calls the function, and
// releases the lock.
func SyncThen[T, S any](mu sync.Locker, promise Promise[T], fn func(T) Result[S]) Promise[S] {
	getPC(&pcSyncThen)
	return Call(func() Result[S] {
		v, err := promise.Result().Get()
		if err != nil {
			return Error[S]{err}
		}

		mu.Lock()
		defer mu.Unlock()
		return fn(v)
	})
}

// getPC stores the program counter of the function that calls it in the atomic
// pointer. getPC does nothing if the pointer already has a value.
func getPC(ptr *atomic.Uintptr) {
	if ptr.Load() != 0 {
		return
	}

	var callers [1]uintptr
	if runtime.Callers(1, callers[:]) == 0 {
		return
	}
	ptr.CompareAndSwap(0, callers[0])
}
