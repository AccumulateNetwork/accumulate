//go:build !mainnet
// +build !mainnet

package block

import "gitlab.com/accumulatenetwork/accumulate/internal/chain"

func addTestnetExecutors(x []chain.TransactionExecutor) []chain.TransactionExecutor {
	return append(x,
		chain.AcmeFaucet{},
	)
}
