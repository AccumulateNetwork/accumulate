// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// SightedAccount is the record's account with each sequence ledger stream's
// Received filled in from Sighted — how far the serving node has sighted that
// stream, which is derived from its staging and is not stored (#4189).
//
// It is merged HERE, by the reader, and not by the server. A server that
// filled it into the body would serve a body that no longer hashes to the
// leaf the receipt in the same response proves, which is what stopped every
// restarted node rejoining (#4295). A reader that wants the number merges it
// after it has checked whatever proof it came with.
//
// The account is copied, so the record is left as it was served.
func (r *AccountRecord) SightedAccount() protocol.Account {
	if r == nil {
		return nil
	}
	if r.Account == nil || len(r.Sighted) == 0 {
		return r.Account
	}

	var seq []*protocol.PartitionSyntheticLedger
	var account protocol.Account
	switch l := r.Account.(type) {
	case *protocol.SyntheticLedger:
		l = l.Copy()
		seq, account = l.Sequence, l
	case *protocol.AnchorLedger:
		l = l.Copy()
		seq, account = l.Sequence, l
	default:
		return r.Account
	}

	bySource := make(map[string]uint64, len(r.Sighted))
	for _, s := range r.Sighted {
		if s != nil && s.Source != nil {
			bySource[strings.ToLower(s.Source.String())] = s.Received
		}
	}
	for _, part := range seq {
		if part.Url == nil {
			continue
		}
		if n, ok := bySource[strings.ToLower(part.Url.String())]; ok {
			part.Received = n
		}
	}
	return account
}
