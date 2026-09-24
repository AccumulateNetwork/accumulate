// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"strings"

	sortutil "gitlab.com/accumulatenetwork/accumulate/internal/util/sort"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func (l *SystemLedger) SetBvnExecutorVersion(bvn string, ver ExecutorVersion) {
	for _, b := range l.BvnExecutorVersions {
		if strings.EqualFold(b.Partition, bvn) {
			if ver > b.Version {
				b.Version = ver
			}
			return
		}
	}
	l.BvnExecutorVersions = append(l.BvnExecutorVersions, &PartitionExecutorVersion{
		Partition: bvn,
		Version:   ver,
	})
}

type SequenceLedger interface {
	Account
	// Partition finds or CREATES the entry for a partition. It writes the
	// record it is called on; a reader uses FindPartition.
	Partition(url *url.URL) *PartitionSyntheticLedger
	// FindPartition finds the entry for a partition without creating one.
	FindPartition(url *url.URL) (*PartitionSyntheticLedger, bool)
}

// findSequence is a read-only lookup in a sorted sequence ledger.
func findSequence(seq []*PartitionSyntheticLedger, url *url.URL) (*PartitionSyntheticLedger, bool) {
	i, found := sortutil.Search(seq, func(entry *PartitionSyntheticLedger) int {
		return entry.Url.Compare(url)
	})
	if !found {
		return nil, false
	}
	return seq[i], true
}

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

func (s *AnchorLedger) Partition(url *url.URL) *PartitionSyntheticLedger {
	return s.Anchor(url)
}

// FindPartition finds the synthetic ledger entry for a partition without
// creating one. The record from a batch is the batch's own memoized value,
// so Partition on it inserts into hashed state; a reader uses this.
func (s *SyntheticLedger) FindPartition(url *url.URL) (*PartitionSyntheticLedger, bool) {
	return findSequence(s.Sequence, url)
}

// FindPartition finds the anchor ledger entry for a partition without
// creating one.
func (s *AnchorLedger) FindPartition(url *url.URL) (*PartitionSyntheticLedger, bool) {
	return findSequence(s.Sequence, url)
}
