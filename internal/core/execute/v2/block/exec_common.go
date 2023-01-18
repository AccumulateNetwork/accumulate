// Copyright 2023 The Accumulate Authors
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
	Type() T
	Process(*bundle, *database.Batch, V) (*protocol.TransactionStatus, error)
}

type MessageExecutor = ExecutorFor[messaging.MessageType, messaging.Message]

func newExecutorMap[T comparable, V interface{ Type() T }](opts ExecutorOptions, list []func(ExecutorOptions) ExecutorFor[T, V]) map[T]ExecutorFor[T, V] {
	m := map[T]ExecutorFor[T, V]{}
	for _, fn := range list {
		x := fn(opts)
		if _, ok := m[x.Type()]; ok {
			panic(errors.InternalError.WithFormat("duplicate executor for %v", x.Type()))
		}
		m[x.Type()] = x
	}
	return m
}
