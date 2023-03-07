// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestCreateKeyPage_HashSize(t *testing.T) {
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)

	// Execute
	st := sim.SubmitTxn(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("book")).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestamp(1).
			WithBody(&CreateKeyPage{Keys: []*KeySpecParams{{KeyHash: []byte{1}}}}).
			Initiate(SignatureTypeED25519, aliceKey).
			Build())
	require.EqualError(t, st.AsError(), "public key hash length is invalid")
}
