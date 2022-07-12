package indexing

import (
	. "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
)

// TransactionChainIndexer indexes account chains against a transaction.
type TransactionChainIndexer struct {
	*record.Set[*TransactionChainEntry]
}

// TransactionChain returns a transaction chain indexer.
func TransactionChain(batch *Batch, txid []byte) *TransactionChainIndexer {
	return &TransactionChainIndexer{batch.Transaction(txid).Chains()}
}
