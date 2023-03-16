// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bsn

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type ExecutorFor[T any, V interface{ Type() T }] interface {
	Process(*Block, V) (*protocol.TransactionStatus, error)
	Validate(*Block, V) (*protocol.TransactionStatus, error)
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

type MessageContext struct {
	message messaging.Message
}

func (m *MessageContext) Type() messaging.MessageType { return m.message.Type() }
