// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package clientsrc adapts a remote v3 client to the
// orchestrator.Source surface.
//
// orchestrator.Source is the union of:
//   - pull.Source         — QueryAccount, QueryDirectoryUrls,
//                           QueryPendingIds, QueryAccountChains,
//                           QueryChainEntries
//   - enumerate.Source    — QueryBptPage
//   - AnchorSource        — LatestAnchor
//
// api.Querier2 already implements every method of pull.Source and
// enumerate.Source with the exact signatures the orchestrator
// requires. The only thing the adapter adds is composing a Querier2
// with an AnchorSource (the trust-decision piece, supplied
// separately per #3988) into a single value the orchestrator can
// consume.
package clientsrc

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/orchestrator"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
)

// Source composes a v3 Querier2 with an AnchorSource. The embedded
// Querier2 satisfies pull.Source and enumerate.Source automatically;
// the embedded AnchorSource satisfies the third half.
type Source struct {
	api.Querier2
	orchestrator.AnchorSource
}

// New builds a Source from a Querier and an AnchorSource. The
// Querier is wrapped in Querier2; pass the same Querier value the
// rest of the launcher uses (e.g., the v3 message-routing client).
func New(q api.Querier, anchors orchestrator.AnchorSource) *Source {
	return &Source{
		Querier2:     api.Querier2{Querier: q},
		AnchorSource: anchors,
	}
}

// Compile-time proof that *Source satisfies orchestrator.Source.
var _ orchestrator.Source = (*Source)(nil)
