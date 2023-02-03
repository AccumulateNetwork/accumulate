// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package testing

import (
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"math/big"
	"strings"

	tmcrypto "github.com/tendermint/tendermint/crypto"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type DB = *database.Batch

func GenerateKey(seed ...interface{}) ed25519.PrivateKey {
	h := storage.MakeKey(seed...)
	return ed25519.NewKeyFromSeed(h[:])
}

func GenerateTmKey(seed ...interface{}) tmed25519.PrivKey {
	return tmed25519.PrivKey(GenerateKey(seed...))
}

func ParseUrl(s string) (*url.URL, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, err
	}
	if protocol.AcmeUrl().Equal(u) {
		return u, nil
	}
	if key, _ := protocol.ParseLiteAddress(u); key != nil {
		return u, nil
	}
	if _, err := protocol.ParseLiteDataAddress(u); err == nil {
		return u, nil
	}
	// Fixup URL
	if !strings.HasSuffix(u.Authority, protocol.TLD) {
		u = u.WithAuthority(u.Authority + protocol.TLD)
	}
	return u, nil
}

func BuildTestTokenTxGenTx(sponsor ed25519.PrivateKey, destAddr string, amount uint64) (*messaging.Envelope, error) {
	//use the public key of the bvc to make a sponsor address (this doesn't really matter right now, but need something so Identity of the BVC is good)
	from := AcmeLiteAddressStdPriv(sponsor)

	u, err := ParseUrl(destAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %v", err)
	}

	send := protocol.SendTokens{}
	send.AddRecipient(u, big.NewInt(int64(amount)))

	return NewTransaction().
		WithPrincipal(from).
		WithSigner(from.RootIdentity(), 1).
		WithTimestamp(1).
		WithBody(&send).
		Initiate(protocol.SignatureTypeLegacyED25519, sponsor).
		Build(), nil
}

func CreateLiteTokenAccount(db DB, key tmed25519.PrivKey, tokens float64) error {
	url := AcmeLiteAddressTmPriv(key)
	err := CreateTokenAccount(db, url.String(), protocol.AcmeUrl().String(), tokens, true)
	if err != nil {
		return err
	}
	liteIdUrl := url.RootIdentity()
	return CreateLiteIdentity(db, liteIdUrl.String(), 0)
}

func AddCredits(db DB, account *url.URL, credits float64) error {
	state, err := db.Account(account).GetState()
	if err != nil {
		return err
	}
	switch state.Type() {
	case protocol.AccountTypeLiteIdentity, protocol.AccountTypeKeyPage:
	default:
		return fmt.Errorf("%s does not refer to a lite identity", account.String())
	}

	state.(protocol.AccountWithCredits).CreditCredits(uint64(credits * protocol.CreditPrecision))
	return db.Account(account).PutState(state)
}

func CreateLiteTokenAccountWithCredits(db DB, key tmed25519.PrivKey, tokens, credits float64) error {
	url := AcmeLiteAddressTmPriv(key)
	err := CreateTokenAccount(db, url.String(), protocol.AcmeUrl().String(), tokens, true)
	if err != nil {
		return err
	}
	liteIdUrl := url.RootIdentity()
	return CreateLiteIdentity(db, liteIdUrl.String(), credits)
}

func WriteStates(db DB, chains ...protocol.Account) error {
	txid := sha256.Sum256([]byte("fake txid"))
	urls := make([]*url.URL, len(chains))
	for i, c := range chains {
		urls[i] = c.GetUrl()

		r := db.Account(c.GetUrl())
		err := r.PutState(c)
		if err != nil {
			return err
		}

		chain, err := r.MainChain().Get()
		if err != nil {
			return err
		}

		err = chain.AddEntry(txid[:], true)
		if err != nil {
			return err
		}
	}

	dir := indexing.Directory(db, urls[0].Identity())
	var uarr []*url.URL
	for _, u := range urls {
		if u.IsRootIdentity() {
			continue
		}
		uarr = append(uarr, u)
	}
	err := dir.Add(uarr...)
	if err != nil {
		return err
	}
	return nil
}

func CreateADI(db DB, key tmed25519.PrivKey, urlStr string) error {
	keyHash := sha256.Sum256(key.PubKey().Bytes()) // TODO This is not what create_identity / create_key_page do, nonce will be > 0 also

	identityUrl, err := ParseUrl(urlStr)
	if err != nil {
		return err
	}

	bookUrl := identityUrl.JoinPath("book0")

	ss := new(protocol.KeySpec)
	ss.PublicKeyHash = keyHash[:]

	page := new(protocol.KeyPage)
	page.Url = protocol.FormatKeyPageUrl(bookUrl, 0)
	page.AddKeySpec(ss)
	page.AcceptThreshold = 1
	page.Version = 1

	book := new(protocol.KeyBook)
	book.Url = bookUrl
	book.AddAuthority(bookUrl)
	book.PageCount = 1

	adi := new(protocol.ADI)
	adi.Url = identityUrl
	adi.AddAuthority(bookUrl)

	return WriteStates(db, adi, book, page)
}

func CreateSubADI(db DB, originUrlStr string, urlStr string) error {
	originUrl, err := ParseUrl(originUrlStr)
	if err != nil {
		return err
	}
	identityUrl, err := ParseUrl(urlStr)
	if err != nil {
		return err
	}

	adi := new(protocol.ADI)
	adi.Url = identityUrl
	adi.AddAuthority(originUrl.JoinPath("book0"))

	return WriteStates(db, adi)
}

func CreateAdiWithCredits(db DB, key tmed25519.PrivKey, urlStr string, credits float64) error {
	err := CreateADI(db, key, urlStr)
	if err != nil {
		return err
	}

	u, err := ParseUrl(urlStr)
	if err != nil {
		return err
	}

	return AddCredits(db, u.JoinPath("book0/1"), credits)
}

func CreateLiteIdentity(db DB, accUrl string, credits float64) error {
	u, err := ParseUrl(accUrl)
	if err != nil {
		return err
	}

	var chain protocol.Account
	account := new(protocol.LiteIdentity)
	account.Url = u
	account.CreditBalance = uint64(credits * protocol.CreditPrecision)
	chain = account
	return db.Account(u).PutState(chain)
}

func CreateTokenAccount(db DB, accUrl, tokenUrl string, tokens float64, lite bool) error {
	u, err := ParseUrl(accUrl)
	if err != nil {
		return err
	}

	tu, err := ParseUrl(tokenUrl)
	if err != nil {
		return err
	}

	var chain protocol.Account
	if lite {
		account := new(protocol.LiteTokenAccount)
		account.Url = u
		account.TokenUrl = tu
		account.Balance.SetInt64(int64(tokens * protocol.AcmePrecision))
		chain = account
	} else {
		account := new(protocol.TokenAccount)
		account.Url = u
		account.TokenUrl = tu
		account.AddAuthority(u.Identity().JoinPath("book0"))
		account.Balance.SetInt64(int64(tokens * protocol.AcmePrecision))
		chain = account
	}

	return db.Account(u).PutState(chain)
}

func CreateTokenIssuer(db DB, urlStr, symbol string, precision uint64, supplyLimit *big.Int) error {
	u, err := ParseUrl(urlStr)
	if err != nil {
		return err
	}

	issuer := new(protocol.TokenIssuer)
	issuer.Url = u
	issuer.AddAuthority(u.Identity().JoinPath("book0"))
	issuer.Symbol = symbol
	issuer.Precision = precision
	issuer.SupplyLimit = supplyLimit

	return db.Account(u).PutState(issuer)
}

func CreateKeyPage(db DB, bookUrlStr string, keys ...tmed25519.PubKey) error {
	bookUrl, err := ParseUrl(bookUrlStr)
	if err != nil {
		return err
	}

	account := db.Account(bookUrl)
	state, err := account.GetState()
	if err != nil {
		return err
	}
	book := state.(*protocol.KeyBook)

	page := new(protocol.KeyPage)
	page.Url = protocol.FormatKeyPageUrl(bookUrl, book.PageCount)
	page.AcceptThreshold = 1
	page.Version = 1
	page.Keys = make([]*protocol.KeySpec, 0, len(keys))
	for _, key := range keys {
		hash := sha256.Sum256(key.Bytes())
		page.AddKeySpec(&protocol.KeySpec{
			PublicKeyHash: hash[:],
		})
	}
	book.PageCount++
	return WriteStates(db, page, book)
}

func CreateKeyBook(db DB, urlStr string, publicKey tmed25519.PubKey) error {
	bookUrl, err := ParseUrl(urlStr)
	if err != nil {
		return err
	}

	book := new(protocol.KeyBook)
	book.Url = bookUrl
	book.AddAuthority(bookUrl)
	book.PageCount = 1

	page := new(protocol.KeyPage)
	page.Version = 1
	page.Url = protocol.FormatKeyPageUrl(bookUrl, 0)

	key := new(protocol.KeySpec)
	hash := sha256.Sum256(publicKey)
	key.PublicKeyHash = hash[:]
	page.Keys = []*protocol.KeySpec{key}

	accounts := []protocol.Account{book, page}
	return WriteStates(db, accounts...)
}

func CreateAccount(db DB, account protocol.FullAccount) error {
	auth := account.GetAuth()
	if len(auth.Authorities) > 0 {
		return WriteStates(db, account)
	}

	var identity *protocol.ADI
	err := db.Account(account.GetUrl().Identity()).GetStateAs(&identity)
	if err != nil {
		return err
	}

	*auth = *identity.GetAuth()
	return WriteStates(db, account)
}

func UpdateKeyPage(db DB, account *url.URL, fn func(*protocol.KeyPage)) error {
	return UpdateAccount(db, account, fn)
}

func UpdateAccount[T protocol.Account](db DB, accountUrl *url.URL, fn func(T)) error {
	var account T
	err := db.Account(accountUrl).GetStateAs(&account)
	if err != nil {
		return err
	}

	fn(account)
	return db.Account(accountUrl).PutState(account)
}

// AcmeLiteAddress creates an ACME lite address for the given key. FOR TESTING
// USE ONLY.
func AcmeLiteAddress(pubKey []byte) *url.URL {
	u, err := protocol.LiteTokenAddress(pubKey, protocol.ACME, protocol.SignatureTypeED25519)
	if err != nil {
		// LiteTokenAddress should only return an error if the token URL is invalid,
		// so this should never return an error. But ignoring errors is a great
		// way to get bugs.
		panic(err)
	}
	return u
}

func AcmeLiteAddressTmPriv(key tmcrypto.PrivKey) *url.URL {
	return AcmeLiteAddress(key.PubKey().Bytes())
}

func AcmeLiteAddressStdPriv(key ed25519.PrivateKey) *url.URL {
	return AcmeLiteAddress(key[32:])
}
