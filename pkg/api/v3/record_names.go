// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// RecordNames is the accounts a block-ledger record's entries name, in entry
// order, repeats included, that a ChangedAccountStates answer serves: every
// entry's account but the placeholder acc://unknown and a name with no
// authority, which name no account (#4461).
func RecordNames(entries []*protocol.BlockEntry) []*url.URL {
	unknown := protocol.UnknownUrl()
	var out []*url.URL
	for _, e := range entries {
		if e == nil || e.Account == nil || e.Account.Authority == "" ||
			e.Account.Equal(unknown) || e.Account.Identity().Equal(unknown) {
			continue
		}
		out = append(out, e.Account)
	}
	return out
}
