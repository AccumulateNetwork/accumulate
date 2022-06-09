package protocol

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/sortutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

// Partition finds or creates a ledger entry for the given partition.
func (s *SyntheticLedger) Partition(url *url.URL) *PartitionSyntheticLedger {
	ptr, create := sortutil.BinaryInsert(&s.Partitions, func(entry *PartitionSyntheticLedger) int {
		return entry.Url.Compare(url)
	})
	if create {
		*ptr = &PartitionSyntheticLedger{Url: url}
	}
	return *ptr
}
