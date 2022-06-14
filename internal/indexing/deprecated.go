package indexing

import (
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	dbv1 "gitlab.com/accumulatenetwork/accumulate/internal/database/v1"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type ChainUpdate = database.ChainUpdate
type BlockStateSynthTxnEntry = database.BlockStateSynthTxnEntry
type TransactionChainEntry = database.TransactionChainEntry

func Directory(batch *dbv1.Batch, account *url.URL) *record.Counted[*url.URL] {
	return batch.Account(account).Directory()
}

func Data(batch *dbv1.Batch, account *url.URL) *database.AccountData {
	return batch.Account(account).Data()
}

func BlockChainUpdates(batch *dbv1.Batch, network *config.Describe, blockIndex uint64) *record.Set[*ChainUpdate] {
	return batch.Account(network.NodeUrl()).BlockChainUpdates()
}

func ProducedSyntheticTransactions(batch *dbv1.Batch, ledger *url.URL) *record.Set[*BlockStateSynthTxnEntry] {
	return batch.Account(ledger).ProducedSyntheticTransactions()
}

func TransactionChain(batch *dbv1.Batch, txid []byte) *record.Set[*TransactionChainEntry] {
	return batch.Transaction(txid).Chains()
}

func GetDataEntry(batch *database.ChangeSet, txnHash []byte) (protocol.DataEntry, error) {
	return database.GetDataEntry(batch, txnHash)
}
