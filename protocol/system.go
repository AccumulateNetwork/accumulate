package protocol

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/sortutil"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

type TransactionExchangeLedgerAccount interface {
	Account
	Partition(url *url.URL) *TransactionExchangeLedger
}

// Partition finds or creates a synthetic ledger entry for the given partition.
func (s *SyntheticLedger) Partition(url *url.URL) *TransactionExchangeLedger {
	ptr, create := sortutil.BinaryInsert(&s.Exchange, func(entry *TransactionExchangeLedger) int {
		return entry.Url.Compare(url)
	})
	if create {
		*ptr = &TransactionExchangeLedger{Url: url}
	}
	return *ptr
}

// Anchor finds or creates an anchor ledger entry for the given partition.
func (s *AnchorLedger) Partition(url *url.URL) *TransactionExchangeLedger {
	ptr, create := sortutil.BinaryInsert(&s.Exchange, func(entry *TransactionExchangeLedger) int {
		return entry.Url.Compare(url)
	})
	if create {
		*ptr = &TransactionExchangeLedger{Url: url}
	}
	return *ptr
}

func (a *PartitionAnchor) SynthFrom(url *url.URL) *TransactionExchangeLedger {
	i, found := sortutil.Search(a.Synthetic, func(entry *TransactionExchangeLedger) int {
		return entry.Url.Compare(url)
	})
	if !found {
		return new(TransactionExchangeLedger)
	}
	return a.Synthetic[i]
}
