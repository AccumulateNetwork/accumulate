// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import "gitlab.com/accumulatenetwork/accumulate/pkg/url"

// A CollectedBlock is what taking a committed block into staging without
// executing it yields (executor spec, "Sync", step 1): how many entries the
// block added to staging, and the accounts its envelopes name.
//
// The accounts are for the state pull (#4293): a joining node pulls what the
// blocks it is collecting touch, rather than enumerating the whole tree
// twice. Nothing else uses them, and nothing is written for them — a
// collected block writes nothing at all.
type CollectedBlock struct {
	// Held is how many entries this block added to staging.
	Held int

	// Accounts are the accounts the block's envelopes name — every
	// transaction's principal, every signer, every anchor's pool — in URL
	// order, without duplicates.
	Accounts []*url.URL
}
