// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package private

import execute "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/multi"

type Executor = execute.Executor
type Dispatcher = execute.Dispatcher
type ExecutorOptions = execute.Options
type Block = execute.Block
type BlockParams = execute.BlockParams
type BlockState = execute.BlockState
type ValidatorUpdate = execute.ValidatorUpdate

func NewExecutor(opts ExecutorOptions) (Executor, error) { return execute.NewExecutor(opts) }
