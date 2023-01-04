// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
)

// Alias these types to minimize imports

type Executor = execute.Executor
type Block = execute.Block
type BlockParams = execute.BlockParams
type BlockState = execute.BlockState
type Options = execute.Options

func NewExecutor(opts Options) (Executor, error) {
	exec, err := block.NewNodeExecutor(opts)
	if err != nil {
		return nil, err
	}
	return (*ExecutorV1)(exec), nil
}
