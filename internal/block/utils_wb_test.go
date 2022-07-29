package block

import "gitlab.com/accumulatenetwork/accumulate/internal/chain"

// White-box testing utilities

func (x *Executor) SetExecutor(y chain.TransactionExecutor) {
	x.executors[y.Type()] = y
}
