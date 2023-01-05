// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/sortutil"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func (e *BlockEntry) Compare(f *BlockEntry) int {
	c := e.Account.Compare(f.Account)
	if c != 0 {
		return c
	}
	c = strings.Compare(strings.ToLower(e.Chain), strings.ToLower(f.Chain))
	if c != 0 {
		return c
	}
	return int(e.Index) - int(f.Index)
}

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
