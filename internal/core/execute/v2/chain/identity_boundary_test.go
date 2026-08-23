// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// #4143: every account a transaction writes shares the principal's root
// identity. These tests cover the assertion itself; the coverage that
// matters is the existing executor and e2e suites run with
// ACC_ASSERT_IDENTITY_BOUNDARY=1, where a violation in any of the ~40
// transaction executors surfaces as a failed test.

func withIdentityBoundary(t *testing.T) {
	t.Helper()
	prev := AssertIdentityBoundary
	AssertIdentityBoundary = true
	t.Cleanup(func() { AssertIdentityBoundary = prev })
}

// boundaryStateManager builds a StateManager for a transaction of the given
// type against the given principal, over a database that already holds the
// principal's identity.
func boundaryStateManager(t *testing.T, txType protocol.TransactionBody, principal protocol.Account) *StateManager {
	t.Helper()
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	t.Cleanup(batch.Discard)
	require.NoError(t, batch.Account(principal.GetUrl()).Main().Put(principal))

	txn := new(protocol.Transaction)
	txn.Header.Principal = principal.GetUrl()
	txn.Body = txType

	shim := execute.DescribeShim{NetworkType: protocol.PartitionTypeBlockValidator, PartitionId: "BVN0"}
	st := NewStateManager(shim, &core.GlobalValues{ExecutorVersion: protocol.ExecutorVersionLatest}, nil, batch, principal, txn, nil)
	t.Cleanup(st.Discard)
	return st
}

func aliceTokens() *protocol.TokenAccount {
	return &protocol.TokenAccount{
		Url:      protocol.AccountUrl("alice", "tokens"),
		TokenUrl: protocol.AcmeUrl(),
		Balance:  *big.NewInt(100),
	}
}

func TestIdentityBoundary_PrincipalWriteAllowed(t *testing.T) {
	withIdentityBoundary(t)
	acct := aliceTokens()
	st := boundaryStateManager(t, &protocol.SendTokens{}, acct)

	acct.Balance = *big.NewInt(50)
	assert.NoError(t, st.Update(acct), "writing the principal itself is always legal")
}

func TestIdentityBoundary_SiblingUnderSameIdentityAllowed(t *testing.T) {
	withIdentityBoundary(t)
	st := boundaryStateManager(t, &protocol.SendTokens{}, aliceTokens())

	// The parent-directory shape from account creation: another account under
	// the same root identity.
	sibling := &protocol.TokenAccount{Url: protocol.AccountUrl("alice", "savings"), TokenUrl: protocol.AcmeUrl()}
	require.NoError(t, st.batch.Account(sibling.Url).Main().Put(sibling))
	assert.NoError(t, st.Update(sibling),
		"a sibling under the principal's identity is within the boundary")
}

func TestIdentityBoundary_AccountCreatedUnderThePrincipalAllowed(t *testing.T) {
	withIdentityBoundary(t)
	adi := &protocol.ADI{Url: protocol.AccountUrl("alice")}
	st := boundaryStateManager(t, &protocol.CreateTokenAccount{}, adi)

	created := &protocol.TokenAccount{Url: protocol.AccountUrl("alice", "new-tokens"), TokenUrl: protocol.AcmeUrl()}
	assert.NoError(t, st.Create(created),
		"creating an account under the principal's identity is within the boundary")
}

func TestIdentityBoundary_ForeignIdentityWriteFails(t *testing.T) {
	withIdentityBoundary(t)
	st := boundaryStateManager(t, &protocol.SendTokens{}, aliceTokens())

	foreign := &protocol.TokenAccount{Url: protocol.AccountUrl("bob", "tokens"), TokenUrl: protocol.AcmeUrl()}
	err := st.Update(foreign)
	require.Error(t, err, "a write outside the principal's identity must fail, not commit")
	assert.ErrorContains(t, err, "identity boundary violation")
	assert.True(t, errors.Is(err, errors.InternalError))

	// And nothing was written: the violation is caught before the put.
	_, err = st.batch.Account(foreign.Url).Main().Get()
	assert.True(t, errors.Is(err, errors.NotFound), "the foreign write must not have landed")
}

// The exemption is explicit, not accidental: system transactions are the
// partition's own bookkeeping.
func TestIdentityBoundary_SystemAccountWriteFromASystemTransactionAllowed(t *testing.T) {
	withIdentityBoundary(t)
	ledger := &protocol.SystemLedger{Url: protocol.PartitionUrl("BVN0").JoinPath(protocol.Ledger)}
	st := boundaryStateManager(t, &protocol.SystemGenesis{}, ledger)

	// A system transaction writing an account of a DIFFERENT identity — e.g.
	// genesis writing the ACME issuer — is exempt.
	other := &protocol.TokenAccount{Url: protocol.AccountUrl("other", "tokens"), TokenUrl: protocol.AcmeUrl()}
	assert.NoError(t, st.Update(other),
		"system transactions are exempt — explicitly")
}

// Reads are legal anywhere; only writes are constrained.
func TestIdentityBoundary_ReadOfAForeignIdentityIsNotAViolation(t *testing.T) {
	withIdentityBoundary(t)
	st := boundaryStateManager(t, &protocol.SendTokens{}, aliceTokens())

	// Seed a foreign account directly (not through the state manager).
	foreign := &protocol.TokenAccount{Url: protocol.AccountUrl("bob", "tokens"), TokenUrl: protocol.AcmeUrl()}
	require.NoError(t, st.batch.Account(foreign.Url).Main().Put(foreign))

	var loaded *protocol.TokenAccount
	require.NoError(t, st.LoadUrlAs(foreign.Url, &loaded),
		"reading a foreign identity is not a violation")
	assert.True(t, loaded.Url.Equal(foreign.Url))
}

// Off by default: the assertion is a CI mode, not a consensus rule — a
// network of assertion-on and assertion-off nodes must not diverge because
// of it, so production behaviour is unchanged until it is deliberately
// enabled everywhere.
func TestIdentityBoundary_DisabledByDefaultChangesNothing(t *testing.T) {
	prev := AssertIdentityBoundary
	AssertIdentityBoundary = false
	t.Cleanup(func() { AssertIdentityBoundary = prev })

	st := boundaryStateManager(t, &protocol.SendTokens{}, aliceTokens())
	foreign := &protocol.TokenAccount{Url: protocol.AccountUrl("bob", "tokens"), TokenUrl: protocol.AcmeUrl()}
	require.NoError(t, st.batch.Account(foreign.Url).Main().Put(foreign))
	assert.NoError(t, st.Update(foreign),
		"with the assertion off, behaviour is exactly what it was")
}

// The one legacy crossing, discovered by running the suites with the
// assertion on: before Vandenberg, markAcmeBurnt writes the partition's own
// system ledger from a user transaction. Kept executable for replay, so the
// exemption is pinned — narrowly, by version and account.
func TestIdentityBoundary_PreVandenbergAcmeBurntLedgerWriteAllowed(t *testing.T) {
	withIdentityBoundary(t)
	acct := aliceTokens()

	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	t.Cleanup(batch.Discard)
	require.NoError(t, batch.Account(acct.GetUrl()).Main().Put(acct))
	ledger := &protocol.SystemLedger{Url: protocol.PartitionUrl("BVN0").JoinPath(protocol.Ledger)}
	require.NoError(t, batch.Account(ledger.Url).Main().Put(ledger))

	txn := new(protocol.Transaction)
	txn.Header.Principal = acct.GetUrl()
	txn.Body = &protocol.AddCredits{}
	shim := execute.DescribeShim{NetworkType: protocol.PartitionTypeBlockValidator, PartitionId: "BVN0"}

	// Pre-Vandenberg: the ledger write is exempt.
	pre := NewStateManager(shim, &core.GlobalValues{ExecutorVersion: protocol.ExecutorVersionV2Baikonur}, nil, batch, acct, txn, nil)
	t.Cleanup(pre.Discard)
	assert.NoError(t, pre.Update(ledger),
		"pre-Vandenberg markAcmeBurnt writes the partition ledger from a user transaction — exempt for replay")

	// At Vandenberg and later the write is a violation again: the burn lives
	// in block state, and nothing else may touch the ledger from a user txn.
	post := NewStateManager(shim, &core.GlobalValues{ExecutorVersion: protocol.ExecutorVersionV2Vandenberg}, nil, batch, acct, txn, nil)
	t.Cleanup(post.Discard)
	assert.ErrorContains(t, post.Update(ledger), "identity boundary violation",
		"the exemption must not outlive the version that needed it")
}
