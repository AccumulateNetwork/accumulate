// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bsn

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

type ExecutorFor[T any, V interface{ Type() T }] interface {
	Process(*ChangeSet, V) error
	Validate(*ChangeSet, V) error
}

type MessageExecutor = ExecutorFor[messaging.MessageType, *MessageContext]

// newExecutorMap creates a map of type to executor from a list of constructors.
func newExecutorMap[T comparable, V interface{ Type() T }](opts ExecutorOptions, list []func(ExecutorOptions) (T, ExecutorFor[T, V])) map[T]ExecutorFor[T, V] {
	m := map[T]ExecutorFor[T, V]{}
	for _, fn := range list {
		typ, x := fn(opts)
		if _, ok := m[typ]; ok {
			panic(errors.InternalError.WithFormat("duplicate executor for %v", typ))
		}
		m[typ] = x
	}
	return m
}

// registerSimpleExec creates a list of constructors for a struct{} executor type and the given list of types.
func registerSimpleExec[X ExecutorFor[T, V], T any, V interface{ Type() T }](list *[]func(ExecutorOptions) (T, ExecutorFor[T, V]), typ ...T) {
	for _, typ := range typ {
		typ := typ // See docs/developer/rangevarref.md
		*list = append(*list, func(ExecutorOptions) (T, ExecutorFor[T, V]) {
			var x X
			return typ, x
		})
	}
}
