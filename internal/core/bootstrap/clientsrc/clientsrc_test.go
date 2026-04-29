// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package clientsrc

import (
	"context"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/orchestrator"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// nopQuerier is a no-op Querier; we don't exercise the methods,
// only the type composition.
type nopQuerier struct{}

func (nopQuerier) Query(_ context.Context, _ *url.URL, _ api.Query) (api.Record, error) {
	return nil, nil
}

// nopAnchorSource is a no-op AnchorSource for the same reason.
type nopAnchorSource struct{}

func (nopAnchorSource) LatestAnchor(_ context.Context, _ string) (uint64, [32]byte, error) {
	return 0, [32]byte{}, nil
}

// TestSource_SatisfiesOrchestrator is a compile-time + runtime
// confirmation that the adapter slots into orchestrator.Source.
// If this fails to compile, the orchestrator interface drifted.
func TestSource_SatisfiesOrchestrator(t *testing.T) {
	src := New(nopQuerier{}, nopAnchorSource{})
	var _ orchestrator.Source = src
}
