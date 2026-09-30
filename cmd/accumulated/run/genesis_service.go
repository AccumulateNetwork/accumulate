// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"os"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// genesisService serves the genesis snapshot a partition was started from:
// the file this node loads into an empty database (network spec, "How a new
// node gets its genesis"). It is read on each request, because it is asked
// for once per node that joins and holding it in memory would cost for the
// life of the process.
type genesisService struct {
	partition string
	path      string
}

var _ api.GenesisService = genesisService{}

func (g genesisService) Genesis(_ context.Context, opts api.GenesisOptions) ([]byte, error) {
	if !strings.EqualFold(opts.Partition, g.partition) {
		return nil, errors.NotFound.WithFormat("this service is for %s, not %s", g.partition, opts.Partition)
	}
	if g.path == "" {
		return nil, errors.NotFound.WithFormat("this node was not configured with a genesis file for %s", g.partition)
	}
	b, err := os.ReadFile(g.path)
	if err != nil {
		return nil, errors.InternalError.WithFormat("read genesis for %s: %w", g.partition, err)
	}
	return b, nil
}
