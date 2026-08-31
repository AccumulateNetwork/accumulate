// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"crypto/ed25519"
	mrand "math/rand"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// claimUniverse builds a universe of n identities, each with a signer page and
// one mutable page, which is the shape the page actions require.
func claimUniverse(t *testing.T, n int) *universe {
	t.Helper()
	u := newUniverse(mrand.New(mrand.NewSource(1)))
	for i := 0; i < n; i++ {
		adi := url.MustParse("acc://lg-claim-" + itoa(i) + ".acme")
		u.adis = append(u.adis, &identity{
			url: adi,
			books: []*keyBook{{
				url: adi.JoinPath("book1"),
				pages: []*keyPage{
					{url: adi.JoinPath("book1", "1"), keys: []ed25519.PrivateKey{newKey()}, threshold: 1, version: 1},
					{url: adi.JoinPath("book1", "2"), version: 1},
				},
			}},
		})
	}
	return u
}

// The defect, stated as a property: a page's capacity may be handed out only
// as many times as it has room.
//
// Testing for room and then filling it are separated by a network round trip,
// and 16 submitters draw at once — so every submitter that drew the same page
// in that window saw the same free slot. Against a 5-key page that is not a
// narrow race: add-page-key was drawn 21,397 times in run 20260831T060018Z and
// 13,693 of those found the page full when they looked.
func TestClaimPageSlot_NeverOverbooksAPage(t *testing.T) {
	u := claimUniverse(t, 1)
	p := u.adis[0].books[0].pages[1]

	var mu sync.Mutex
	granted := 0
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, got := u.claimPageSlot(maxPageKeys); got != nil {
				mu.Lock()
				granted++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, maxPageKeys, granted,
		"a page with room for %d keys must grant exactly %d claims", maxPageKeys, maxPageKeys)
	assert.Equal(t, maxPageKeys, p.pending, "every granted claim must hold its reservation")
	_, none := u.claimPageSlot(maxPageKeys)
	assert.Nil(t, none, "a fully reserved page must offer nothing more")
}

// A failed attempt returns the slot, or the universe leaks capacity until
// every page looks full and the action can never be drawn again.
func TestClaimPageSlot_ReleaseRestoresCapacity(t *testing.T) {
	u := claimUniverse(t, 1)
	_, p := u.claimPageSlot(maxPageKeys)
	require.NotNil(t, p)
	require.Equal(t, 1, p.pending)

	u.releasePageSlot(p)
	assert.Equal(t, 0, p.pending, "a released claim must free the slot")
	assert.Empty(t, p.keys, "releasing must not record a key")
}

// Committing turns the reservation into a usable key and bumps the version
// UpdateKeyPage incremented on chain.
func TestClaimPageSlot_CommitRecordsTheKey(t *testing.T) {
	u := claimUniverse(t, 1)
	_, p := u.claimPageSlot(maxPageKeys)
	require.NotNil(t, p)
	before := p.version

	k := newKey()
	u.commitPageKey(p, k)
	assert.Equal(t, 0, p.pending, "commit must consume the reservation")
	require.Len(t, p.keys, 1)
	assert.Equal(t, k, p.keys[0])
	assert.Equal(t, before+1, p.version, "UpdateKeyPage bumps the version")
}

// Books are named from their sequence number, so two live claims must never
// carry the same one — deriving it from len(books) is what had two concurrent
// creates racing for .../book2, one of them waiting out three minutes on an
// account the other was already making.
func TestClaimBook_LiveSequenceNumbersAreUnique(t *testing.T) {
	u := claimUniverse(t, 1)
	a := u.adis[0]

	seen := map[int]bool{}
	for i := 0; i < maxBooks-1; i++ { // one book already exists
		got, seq := u.claimBook()
		require.NotNil(t, got, "claim %d should be granted", i)
		assert.False(t, seen[seq], "sequence %d handed out twice", seq)
		seen[seq] = true
	}

	none, _ := u.claimBook()
	assert.Nil(t, none, "the cap of %d books must be respected", maxBooks)
	assert.Equal(t, maxBooks-1, a.pendingBooks)
}

// The collision that a single shared pending counter allows: tokens and data
// name their urls from their OWN lengths, so committing a data account must
// not hand a live token sequence number back out.
func TestClaimAccount_TokenAndDataSequencesDoNotCollide(t *testing.T) {
	u := claimUniverse(t, 1)
	a := u.adis[0]

	// Force one claim of each kind regardless of the coin flip inside
	// claimAccount, by taking claims until both kinds have appeared.
	var dataClaim, tokenClaim *accountClaim
	for dataClaim == nil || tokenClaim == nil {
		c := u.claimAccount()
		require.NotNil(t, c, "the identity has room for %d accounts", maxAccounts)
		if c.data && dataClaim == nil {
			dataClaim = c
		} else if !c.data && tokenClaim == nil {
			tokenClaim = c
		} else {
			u.releaseAccount(c)
		}
	}

	// Commit the data account. A shared counter would drop the token
	// reservation's contribution and reissue its sequence number.
	u.commitAccount(dataClaim, a.url.JoinPath("data"+itoa(dataClaim.seq)))

	live := tokenClaim.seq
	for i := 0; i < 4; i++ {
		c := u.claimAccount()
		require.NotNil(t, c)
		if !c.data {
			assert.NotEqual(t, live, c.seq,
				"token sequence %d is still in flight and must not be reissued", live)
		}
	}
}

// Account capacity is the sum of both kinds, and must be honoured under
// concurrent claims.
func TestClaimAccount_NeverOverbooksAnIdentity(t *testing.T) {
	u := claimUniverse(t, 1)

	var mu sync.Mutex
	granted := 0
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if c := u.claimAccount(); c != nil {
				mu.Lock()
				granted++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, maxAccounts, granted,
		"an identity with room for %d accounts must grant exactly that many", maxAccounts)
	assert.Nil(t, u.claimAccount(), "a fully reserved identity must offer nothing more")
}

// Releasing an account claim restores the capacity of the kind it took.
func TestClaimAccount_ReleaseRestoresTheRightKind(t *testing.T) {
	u := claimUniverse(t, 1)
	a := u.adis[0]

	c := u.claimAccount()
	require.NotNil(t, c)
	require.Equal(t, 1, a.pendingAccounts())

	u.releaseAccount(c)
	assert.Equal(t, 0, a.pendingAccounts(), "release must free the reservation")
	assert.Zero(t, a.pendingToken)
	assert.Zero(t, a.pendingData)
}
