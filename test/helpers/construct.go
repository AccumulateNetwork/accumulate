// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package helpers

import (
	"crypto/sha256"
	"math/big"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func MustBuild(t testing.TB, b interface {
	Done() (*messaging.Envelope, error)
}) *messaging.Envelope {
	t.Helper()
	env, err := b.Done()
	require.NoError(t, err)
	return env
}

func MustGet0[T any](t testing.TB, fn func() (T, error)) T {
	v, err := fn()
	require.NoError(t, err)
	return v
}

func MustGet1[T, A1 any](t testing.TB, fn func(A1) (T, error), a1 A1) T {
	v, err := fn(a1)
	require.NoError(t, err)
	return v
}

func View(t testing.TB, db database.Viewer, fn func(batch *database.Batch)) {
	t.Helper()
	require.NoError(t, db.View(func(batch *database.Batch) error {
		t.Helper()
		fn(batch)
		return nil
	}))
}

func Update(t testing.TB, db database.Updater, fn func(batch *database.Batch)) {
	t.Helper()
	require.NoError(t, db.Update(func(batch *database.Batch) error {
		t.Helper()
		fn(batch)
		return nil
	}))
}

func GetAccount[T protocol.Account](t testing.TB, db database.Viewer, account *url.URL) T {
	t.Helper()
	var v T
	View(t, db, func(batch *database.Batch) {
		t.Helper()
		require.NoError(t, batch.Account(account).Main().GetAs(&v))
	})
	return v
}

// PutAccount writes the account's main state.
func PutAccount(t testing.TB, db database.Updater, account protocol.Account) {
	t.Helper()
	Update(t, db, func(batch *database.Batch) {
		t.Helper()
		require.NoError(t, batch.Account(account.GetUrl()).Main().Put(account))
	})
}

// MakeAccount writes the account's main state, adds a directory entry to the
// parent, and inherits the parent's authority if non is specified.
func MakeAccount(t testing.TB, db database.Updater, account ...protocol.Account) {
	t.Helper()
	require.NoError(t, TryMakeAccount(t, db, account...))
}

// TryMakeAccount writes the account's main state, adds a directory entry to the
// parent, and inherits the parent's authority if non is specified.
func TryMakeAccount(t testing.TB, db database.Updater, account ...protocol.Account) error {
	t.Helper()
	return db.Update(func(batch *database.Batch) error {
		t.Helper()
		for _, account := range account {
			// Write state
			u := account.GetUrl()
			err := batch.Account(u).Main().Put(account)
			if err != nil {
				return err
			}

			if u.IsRootIdentity() {
				continue
			}

			// Add directory entry
			err = batch.Account(u.Identity()).Directory().Add(u)
			if err != nil {
				return err
			}

			full, ok := account.(protocol.FullAccount)
			if !ok || len(full.GetAuth().Authorities) > 0 {
				continue
			}

			// Inherit the parent's authorities
			var identity *protocol.ADI
			err = batch.Account(u.Identity()).Main().GetAs(&identity)
			if err != nil {
				return err
			}
			*full.GetAuth() = identity.AccountAuth
			err = batch.Account(u).Main().Put(account)
			if err != nil {
				return err
			}
		}

		return nil
	})
}

func UpdateAccount[T protocol.Account](t testing.TB, db database.Updater, account *url.URL, fn func(T)) {
	t.Helper()
	Update(t, db, func(batch *database.Batch) {
		t.Helper()
		var v T
		require.NoError(t, batch.Account(account).Main().GetAs(&v))
		fn(v)
		require.NoError(t, batch.Account(account).Main().Put(v))
	})
}

func CreditCredits(t testing.TB, db database.Updater, account *url.URL, amount uint64) {
	t.Helper()
	UpdateAccount(t, db, account, func(a protocol.AccountWithCredits) {
		t.Helper()
		a.CreditCredits(amount)
	})
}

func CreditTokens(t testing.TB, db database.Updater, account *url.URL, amount *big.Int) {
	t.Helper()
	UpdateAccount(t, db, account, func(a protocol.AccountWithTokens) {
		t.Helper()
		a.CreditTokens(amount)
	})
}

func MakeLiteTokenAccount(t testing.TB, db database.Updater, pubKey []byte, token *url.URL) *url.URL {
	t.Helper()
	lid := protocol.LiteAuthorityForKey(pubKey, protocol.SignatureTypeED25519)
	lta := lid.JoinPath(token.ShortString())
	MakeAccount(t, db, &protocol.LiteIdentity{Url: lid})
	MakeAccount(t, db, &protocol.LiteTokenAccount{Url: lta, TokenUrl: token})
	return lta
}

func MakeIdentity(t testing.TB, db database.Updater, u *url.URL, pubKeys ...[]byte) {
	t.Helper()
	identity := new(protocol.ADI)
	identity.Url = u
	identity.AddAuthority(u.JoinPath("book"))

	book, page := newBook(u.JoinPath("book"), pubKeys...)
	MakeAccount(t, db, identity, book, page)
}

func MakeKeyBook(t testing.TB, db database.Updater, u *url.URL, pubKeys ...[]byte) {
	t.Helper()
	book, page := newBook(u, pubKeys...)
	MakeAccount(t, db, book, page)
}

func newBook(u *url.URL, pubKeys ...[]byte) (*protocol.KeyBook, *protocol.KeyPage) {
	book := new(protocol.KeyBook)
	book.Url = u
	book.AddAuthority(u)
	book.PageCount = 1

	page := newPage(u.JoinPath("1"), pubKeys...)
	return book, page
}

func MakeKeyPage(t testing.TB, db database.Updater, book *url.URL, pubKeys ...[]byte) {
	t.Helper()
	var n string
	UpdateAccount(t, db, book, func(book *protocol.KeyBook) {
		t.Helper()
		book.PageCount++
		n = strconv.FormatUint(book.PageCount, 10)
	})

	page := newPage(book.JoinPath(n), pubKeys...)
	MakeAccount(t, db, page)
}

func newPage(u *url.URL, pubKeys ...[]byte) *protocol.KeyPage {
	page := new(protocol.KeyPage)
	page.Url = u
	page.AcceptThreshold = 1
	page.Version = 1

	for _, pubKey := range pubKeys {
		if len(pubKey) != 32 {
			panic("expected 32 byte public key")
		}
		keyHash := sha256.Sum256(pubKey)
		key := new(protocol.KeySpec)
		key.PublicKeyHash = keyHash[:]
		page.AddKeySpec(key)
	}
	return page
}
