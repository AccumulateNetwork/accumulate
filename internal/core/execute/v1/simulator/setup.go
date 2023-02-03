// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"crypto/sha256"
	"math/big"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func writeAccountState(t TB, batch *database.Batch, account protocol.Account) {
	record := batch.Account(account.GetUrl())
	require.NoError(tb{t}, record.PutState(account))

	txid := sha256.Sum256([]byte("fake txid"))
	mainChain, err := record.MainChain().Get()
	require.NoError(tb{t}, err)
	require.NoError(tb{t}, mainChain.AddEntry(txid[:], true))

	identity, ok := account.GetUrl().Parent()
	if ok {
		require.NoError(tb{t}, indexing.Directory(batch, identity).Add(account.GetUrl()))
	}
}

func (s *Simulator) CreateAccount(account protocol.Account) {
	_ = s.PartitionFor(account.GetUrl()).Database.Update(func(batch *database.Batch) error {
		full, ok := account.(protocol.FullAccount)
		if !ok {
			writeAccountState(s, batch, account)
			return nil
		}

		auth := full.GetAuth()
		if len(auth.Authorities) > 0 {
			writeAccountState(s, batch, account)
			return nil
		}

		identityUrl, ok := account.GetUrl().Parent()
		require.True(tb{s}, ok, "Attempted to create an account with no auth")

		var identity *protocol.ADI
		require.NoError(tb{s}, batch.Account(identityUrl).GetStateAs(&identity))

		*auth = identity.AccountAuth
		writeAccountState(s, batch, account)
		return nil
	})
}

func (s *Simulator) CreateLiteTokenAccount(key []byte, token *url.URL, credits, tokens uint64) *url.URL {
	lid := protocol.LiteAuthorityForKey(key[32:], protocol.SignatureTypeED25519)
	lta := lid.JoinPath(token.ShortString())
	s.CreateAccount(&protocol.LiteIdentity{Url: lid, CreditBalance: credits})
	s.CreateAccount(&protocol.LiteTokenAccount{Url: lta, TokenUrl: token, Balance: *big.NewInt(int64(tokens))})
	return lta
}

func (s *Simulator) CreateIdentity(identityUrl *url.URL, pubKey ...[]byte) {
	_ = s.PartitionFor(identityUrl).Database.Update(func(batch *database.Batch) error {
		identity := new(protocol.ADI)
		identity.Url = identityUrl
		identity.AddAuthority(identityUrl.JoinPath("book"))

		book := new(protocol.KeyBook)
		book.Url = identityUrl.JoinPath("book")
		book.AddAuthority(identityUrl.JoinPath("book"))
		book.PageCount = 1

		page := new(protocol.KeyPage)
		page.Url = protocol.FormatKeyPageUrl(identityUrl.JoinPath("book"), 0)
		page.AcceptThreshold = 1
		page.Version = 1

		for _, pubKey := range pubKey {
			keyHash := sha256.Sum256(pubKey)
			key := new(protocol.KeySpec)
			key.PublicKeyHash = keyHash[:]
			page.AddKeySpec(key)
		}

		writeAccountState(s, batch, identity)
		writeAccountState(s, batch, book)
		writeAccountState(s, batch, page)
		return nil
	})
}

func (s *Simulator) CreateKeyBook(bookUrl *url.URL, pubKey ...[]byte) {
	_ = s.PartitionFor(bookUrl).Database.Update(func(batch *database.Batch) error {
		book := new(protocol.KeyBook)
		book.Url = bookUrl
		book.AddAuthority(bookUrl)
		book.PageCount = 1

		page := new(protocol.KeyPage)
		page.Url = protocol.FormatKeyPageUrl(bookUrl, 0)
		page.AcceptThreshold = 1
		page.Version = 1

		for _, pubKey := range pubKey {
			keyHash := sha256.Sum256(pubKey)
			key := new(protocol.KeySpec)
			key.PublicKeyHash = keyHash[:]
			page.AddKeySpec(key)
		}

		writeAccountState(s, batch, book)
		writeAccountState(s, batch, page)
		return nil
	})
}

func (s *Simulator) CreateKeyPage(bookUrl *url.URL, pubKey ...[]byte) {
	_ = s.PartitionFor(bookUrl).Database.Update(func(batch *database.Batch) error {
		var book *protocol.KeyBook
		require.NoError(tb{s}, batch.Account(bookUrl).GetStateAs(&book))
		pageUrl := protocol.FormatKeyPageUrl(bookUrl, book.PageCount)
		book.PageCount++

		page := new(protocol.KeyPage)
		page.Url = pageUrl
		page.AcceptThreshold = 1
		page.Version = 1

		for _, pubKey := range pubKey {
			keyHash := sha256.Sum256(pubKey)
			key := new(protocol.KeySpec)
			key.PublicKeyHash = keyHash[:]
			page.AddKeySpec(key)
		}

		writeAccountState(s, batch, book)
		writeAccountState(s, batch, page)
		return nil
	})
}

func (s *Simulator) UpdateAccount(accountUrl *url.URL, fn func(account protocol.Account)) {
	s.Helper()
	_ = s.PartitionFor(accountUrl).Database.Update(func(batch *database.Batch) error {
		s.Helper()
		account, err := batch.Account(accountUrl).GetState()
		require.NoError(tb{s}, err)
		fn(account)
		writeAccountState(s, batch, account)
		return nil
	})
}

func GetAccount[T protocol.Account](sim *Simulator, accountUrl *url.URL) T {
	sim.Helper()
	var account T
	_ = sim.PartitionFor(accountUrl).Database.View(func(batch *database.Batch) error {
		sim.Helper()
		require.NoError(tb{sim}, batch.Account(accountUrl).GetStateAs(&account))
		return nil
	})
	return account
}

func GetTxnState[V any, T interface{ Get() (V, error) }](sim *Simulator, txid *url.TxID, state func(*database.Transaction) T) V {
	sim.Helper()
	var value V
	var err error
	_ = sim.PartitionFor(txid.Account()).Database.View(func(batch *database.Batch) error {
		sim.Helper()
		h := txid.Hash()
		value, err = state(batch.Transaction(h[:])).Get()
		require.NoError(tb{sim}, err)
		return nil
	})
	return value
}
