// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package url_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// #4143 Stage 0: the identity-sharding premise starts from routing being a
// function of the IDENTITY, not the account. Every account under one
// authority routes together, so "every write belongs to the principal's
// identity" implies "every write belongs to this partition" — the property
// the partition boundary enforces today and sharded execution will lean on.

func TestRoutingIsByIdentityNotAccount(t *testing.T) {
	alice := MustParse("alice.acme")
	assert.Equal(t, alice.Routing(), MustParse("alice.acme/tokens").Routing(),
		"an account routes with its identity")
	assert.Equal(t, alice.Routing(), MustParse("alice.acme/book").Routing())
	assert.Equal(t, alice.Routing(), MustParse("alice.acme/book/1").Routing(),
		"depth does not matter — only the authority routes")
	assert.Equal(t, alice.Routing(), MustParse("ALICE.acme/tokens").Routing(),
		"routing is case-insensitive, like the rest of the URL")
}

// A lite token account and its lite identity are ONE identity: acc://<key>
// and acc://<key>/ACME share a root and a routing number, so a deposit to
// the token account is a write within the identity that owns it.
func TestLiteTokenAccountAndLiteIdentityAreSameIdentity(t *testing.T) {
	lid := protocol.LiteAuthorityForKey(make([]byte, 32), protocol.SignatureTypeED25519)
	lta := lid.JoinPath("ACME")

	assert.True(t, lta.RootIdentity().Equal(lid), "the token account's root is the lite identity")
	assert.Equal(t, lid.Routing(), lta.Routing(), "and they route together")
	assert.True(t, lid.LocalTo(lta), "LocalTo agrees — this is the predicate the executor uses today")
}

func TestDistinctIdentitiesGetDistinctRoutingNumbers(t *testing.T) {
	seen := map[uint64]string{}
	for _, name := range []string{"alice.acme", "bob.acme", "charlie.acme", "dn.acme", "bvn-BVN0.acme"} {
		r := MustParse(name).Routing()
		prev, dup := seen[r]
		require.False(t, dup, "%s and %s collide on routing number %d", name, prev, r)
		seen[r] = name
	}
}
