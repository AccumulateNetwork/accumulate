// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build !mainnet
// +build !mainnet

package block

import "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v1/chain"

func addTestnetExecutors(x []chain.TransactionExecutor) []chain.TransactionExecutor {
	return append(x,
		chain.AcmeFaucet{},
	)
}
