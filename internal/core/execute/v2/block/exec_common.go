// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type ExecutorFor[T any, V interface{ Type() T }] interface {
	Process(*database.Batch, V) (*protocol.TransactionStatus, error)
	Validate(*database.Batch, V) (*protocol.TransactionStatus, error)
}

type ExecutorFactory1[T any, V interface{ Type() T }] func(ExecutorOptions) (T, ExecutorFactory2[T, V])
type ExecutorFactory2[T any, V interface{ Type() T }] func(V) (ExecutorFor[T, V], bool)

type MessageExecutor = ExecutorFor[messaging.MessageType, *MessageContext]
type SignatureExecutor = ExecutorFor[protocol.SignatureType, *SignatureContext]

// newExecutorMap creates a map of type to executor from a list of constructors.
func newExecutorMap[T comparable, V interface{ Type() T }](opts ExecutorOptions, list []ExecutorFactory1[T, V]) map[T]ExecutorFactory2[T, V] {
	m := map[T]ExecutorFactory2[T, V]{}
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
func registerSimpleExec[X ExecutorFor[T, V], T any, V interface{ Type() T }](list *[]ExecutorFactory1[T, V], typ ...T) {
	for _, typ := range typ {
		typ := typ // See docs/developer/rangevarref.md
		*list = append(*list, func(ExecutorOptions) (T, ExecutorFactory2[T, V]) {
			return typ, func(V) (ExecutorFor[T, V], bool) {
				var x X
				return x, true
			}
		})
	}
}

func registerConditionalExec[X ExecutorFor[T, V], T any, V interface{ Type() T }](list *[]ExecutorFactory1[T, V], cond func(ctx V) bool, typ ...T) {
	for _, typ := range typ {
		typ := typ // See docs/developer/rangevarref.md
		*list = append(*list, func(ExecutorOptions) (T, ExecutorFactory2[T, V]) {
			return typ, func(ctx V) (ExecutorFor[T, V], bool) {
				var x X
				return x, cond(ctx)
			}
		})
	}
}

func getExecutor[T comparable, V interface{ Type() T }](m map[T]ExecutorFactory2[T, V], ctx V) (ExecutorFor[T, V], bool) {
	f, ok := m[ctx.Type()]
	if !ok {
		return nil, false
	}

	return f(ctx)
}
