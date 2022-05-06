package protocol

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/sortutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

// Subnet finds or creates a ledger entry for the given subnet.
func (s *SyntheticLedger) Subnet(url *url.URL) *SubnetSyntheticLedger {
	ptr, create := sortutil.BinaryInsert(&s.Subnets, func(entry *SubnetSyntheticLedger) int {
		return entry.Url.Compare(url)
	})
	if create {
		*ptr = &SubnetSyntheticLedger{Url: url}
	}
	return *ptr
}
