// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// peerSources is one peer: a real querier over a real store, reached the way
// the join reaches a peer. Its answers are the API's, receipts and all.
type peerSources struct {
	noValidators
	partition *url.URL
	querier   api.Querier
}

func (p *peerSources) For(context.Context, *url.URL) ([]pull.Source, *url.URL, error) {
	return []pull.Source{api.Querier2{Querier: p.querier}}, p.partition, nil
}

func (p *peerSources) Querier(*url.URL) api.Querier { return p.querier }

// noAnchors is a Directory that has executed no anchor, so every block a peer
// serves at is one the join has no root for.
type noAnchors struct{}

func (noAnchors) Query(context.Context, *url.URL, api.Query) (api.Record, error) {
	return nil, errors.NotFound.With("the directory has executed no anchors")
}

// putLedger writes a partition's system ledger at a block.
func putLedger(t *testing.T, db *database.Database, partition *url.URL, block uint64) {
	t.Helper()
	batch := db.Begin(true)
	defer batch.Discard()
	ledger := new(protocol.SystemLedger)
	ledger.Url = partition.JoinPath(protocol.Ledger)
	ledger.Index = block
	require.NoError(t, batch.Account(ledger.Url).Main().Put(ledger))
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())
}
