package protocol

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/sortutil"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// Partition finds or creates a synthetic ledger entry for the given partition.
func (s *SyntheticLedger) Partition(url *url.URL) *PartitionSyntheticLedger {
	ptr, create := sortutil.BinaryInsert(&s.Sequence, func(entry *PartitionSyntheticLedger) int {
		return entry.Url.Compare(url)
	})
	if create {
		*ptr = &PartitionSyntheticLedger{Url: url}
	}
	return *ptr
}

// Anchor finds or creates an anchor ledger entry for the given partition.
func (s *AnchorLedger) Anchor(url *url.URL) *PartitionSyntheticLedger {
	ptr, create := sortutil.BinaryInsert(&s.Sequence, func(entry *PartitionSyntheticLedger) int {
		return entry.Url.Compare(url)
	})
	if create {
		*ptr = &PartitionSyntheticLedger{Url: url}
	}
	return *ptr
}
