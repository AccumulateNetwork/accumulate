// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bsn

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

type indexerFactory = func(ctx *SummaryContext, update *messaging.RecordUpdate, record record.Record, remainingKey *record.Key) error

var indexerFactories []indexerFactory

func registerIndexerFactory(f indexerFactory) {
	indexerFactories = append(indexerFactories, f)
}

type SummaryContext struct {
	*MessageContext

	summary *messaging.BlockSummary

	indexers []indexer
}

type indexer interface {
	Key() *record.Key
	Apply(*ChangeSet, *SummaryContext, record.Record) error
}

func (s *SummaryContext) index(indexer indexer) {
	s.indexers = append(s.indexers, indexer)
}
