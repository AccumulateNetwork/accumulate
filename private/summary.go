// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package private

import "gitlab.com/accumulatenetwork/accumulate/internal/bsn"

type SummaryExecutorOptions = bsn.ExecutorOptions
type SummaryCollectorOptions = bsn.CollectorOptions
type DidCollectBlock = bsn.DidCollectBlock

func NewSummaryExecutor(opts SummaryExecutorOptions) (Executor, error) {
	return bsn.NewExecutor(opts)
}

func StartSummaryCollector(opts SummaryCollectorOptions) (*bsn.Collector, error) {
	return bsn.StartCollector(opts)
}
