// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block_test

import (
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

// From v2-canaveral a user transaction and its signatures name every account
// without user info, a port, a query or a fragment: such a URL names the same
// account as the URL without them and compares unequal to it (executor spec,
// "Account URLs").
func TestAccountUrlAliases(t *testing.T) {
	alice := protocol.AccountUrl("alice")
	aliceKey := generateKey(alice)
	aliased := url.MustParse("acc://user@alice.acme")

	cases := map[string]func(timestamp *uint64) build.SignatureBuilder{
		"principal": func(ts *uint64) build.SignatureBuilder {
			return build.Transaction().For(aliased.JoinPath("tokens")).BurnTokens(1, 0).
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey)
		},
		"signer": func(ts *uint64) build.SignatureBuilder {
			return build.Transaction().For(alice, "tokens").BurnTokens(1, 0).
				SignWith(aliased.JoinPath("book", "1")).Version(1).Timestamp(ts).PrivateKey(aliceKey)
		},
		"recipient": func(ts *uint64) build.SignatureBuilder {
			return build.Transaction().For(alice, "tokens").SendTokens(1, 0).To("acc://bob.acme:1/tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey)
		},
	}

	for _, version := range []protocol.ExecutorVersion{protocol.ExecutorVersionV2Kourou, protocol.ExecutorVersionV2Canaveral} {
		for name, build := range cases {
			t.Run(fmt.Sprintf("%v/%s", version, name), func(t *testing.T) {
				sim := NewSim(t,
					simulator.SimpleNetwork(t.Name(), 2, 1),
					simulator.GenesisWithVersion(GenesisTime, version),
				)
				MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
				CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
				MakeAccount(t, sim.DatabaseFor(alice), &protocol.TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: protocol.AcmeUrl(), Balance: *big.NewInt(1e15)})

				var timestamp uint64
				var refused bool
				for _, st := range sim.BuildAndSubmit(build(&timestamp)) {
					if st.Code == errors.BadRequest && strings.Contains(st.AsError().Error(), "account URL") {
						refused = true
					}
				}
				if version.V2CanaveralEnabled() {
					require.True(t, refused, "the aliased URL must be refused")
				} else {
					require.False(t, refused, "before v2-canaveral the URL is not checked")
				}
			})
		}
	}
}
