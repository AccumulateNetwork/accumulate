// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package pull is the v2 bootstrap data phase: it pulls the complete
// state surface for an account from a Source (the network or a test
// fixture) and writes it into a local database such that
// UpdateBPT() will reproduce the network's leaf hash byte-for-byte.
//
// The contract Pull must satisfy is pinned by the completeness
// package's round-trip test. If the production observer's hashState
// reads a field, Pull must round-trip it.
package pull

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Source is the read-only surface Pull needs from the network. It is
// kept narrow on purpose: a fake backed by a local database is the
// test path; the api.Querier2-backed adapter is the production path.
type Source interface {
	// Main returns the account's main state at the source's current
	// view. Returns (nil, nil) if the account exists but has no main
	// state (rare; pre-genesis placeholders).
	Main(ctx context.Context, u *url.URL) (protocol.Account, error)

	// DirectoryUrls returns the list of sub-account URLs the account
	// directly contains (only populated for ADIs / KeyBooks). Order
	// is the source's natural order; callers should not assume
	// canonical ordering.
	DirectoryUrls(ctx context.Context, u *url.URL) ([]*url.URL, error)

	// PendingIDs returns the txids of pending transactions associated
	// with the account.
	PendingIDs(ctx context.Context, u *url.URL) ([]*url.TxID, error)

	// ChainNames returns the names of every chain on the account.
	// Order matters for byte-equivalent BPT reconstruction: the
	// production observer iterates chains in the order Chains().Get()
	// returns them, which is alphabetical. Sources should preserve
	// that contract.
	ChainNames(ctx context.Context, u *url.URL) ([]string, error)

	// ChainEntries returns every entry in the named chain in chain
	// order (oldest first). Each entry is the raw 32-byte hash that
	// the chain stores.
	ChainEntries(ctx context.Context, u *url.URL, chainName string) ([][]byte, error)
}
