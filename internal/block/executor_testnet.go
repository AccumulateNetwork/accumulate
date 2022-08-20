//go:build !mainnet
// +build !mainnet

package block

import "gitlab.com/accumulatenetwork/accumulate/internal/execute"

func addTestnetExecutors(x []execute.TransactionExecutor) []execute.TransactionExecutor {
	return append(x,
		execute.AcmeFaucet{},
	)
}
