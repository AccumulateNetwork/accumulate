//go:build production
// +build production

package block

import "gitlab.com/accumulatenetwork/accumulate/internal/chain"

func addTestnetExecutors(x []chain.TransactionExecutor) []chain.TransactionExecutor {
	return x
}
