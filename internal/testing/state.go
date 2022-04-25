package testing

import (
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"math/big"

	tmcrypto "github.com/tendermint/tendermint/crypto"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/types"
)

type DB = *database.Batch

var FakeBvn = MustParseUrl("acc://bvn0")

func GenerateKey(seed ...interface{}) ed25519.PrivateKey {
	h := storage.MakeKey(seed...)
	return ed25519.NewKeyFromSeed(h[:])
}

func GenerateTmKey(seed ...interface{}) tmed25519.PrivKey {
	return tmed25519.PrivKey(GenerateKey(seed...))
}

func MustParseUrl(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

func BuildTestTokenTxGenTx(sponsor ed25519.PrivateKey, destAddr string, amount uint64) (*protocol.Envelope, error) {
	//use the public key of the bvc to make a sponsor address (this doesn't really matter right now, but need something so Identity of the BVC is good)
	from := AcmeLiteAddressStdPriv(sponsor)

	u, err := url.Parse(destAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %v", err)
	}

	send := protocol.SendTokens{}
	send.AddRecipient(u, big.NewInt(int64(amount)))

	return NewTransaction().
		WithPrincipal(from).
		WithSigner(from, 1).
		WithTimestamp(1).
		WithBody(&send).
		Initiate(protocol.SignatureTypeLegacyED25519, sponsor).
		Build(), nil
}

func CreateLiteTokenAccount(db DB, key tmed25519.PrivKey, tokens float64) error {
	url := AcmeLiteAddressTmPriv(key).String()
	return CreateTokenAccount(db, string(url), protocol.AcmeUrl().String(), tokens, true)
}

func AddCredits(db DB, account *url.URL, credits float64) error {
	state, err := db.Account(account).GetState()
	if err != nil {
		return err
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

	return AddCredits(db, url, credits)
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

		chain, err := r.Chain(protocol.MainChain, protocol.ChainTypeTransaction)
		if err != nil {
			return err
		}

		err = chain.AddEntry(txid[:], true)
		if err != nil {
			return err
		}
	}

	directory := urls[0].Identity()
	return chain.AddDirectoryEntry(func(u *url.URL, key ...interface{}) chain.Value {
		return db.Account(u).Index(key...)
	}, directory, urls...)
}

func CreateADI(db DB, key tmed25519.PrivKey, urlStr types.String) error {
	keyHash := sha256.Sum256(key.PubKey().Bytes()) // TODO This is not what create_identity / create_key_page do, nonce will be > 0 also

	identityUrl, err := url.Parse(*urlStr.AsString())
	if err != nil {
		return err
	}

	bookUrl := identityUrl.JoinPath("book0")

	ss := new(protocol.KeySpec)
	ss.PublicKeyHash = keyHash[:]

	page := new(protocol.KeyPage)
	page.Url = protocol.FormatKeyPageUrl(bookUrl, 0)
	page.Keys = append(page.Keys, ss)
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

func CreateSubADI(db DB, originUrlStr types.String, urlStr types.String) error {
	originUrl, err := url.Parse(*originUrlStr.AsString())
	if err != nil {
		return err
	}
	identityUrl, err := url.Parse(*urlStr.AsString())
	if err != nil {
		return err
	}

	adi := new(protocol.ADI)
	adi.Url = identityUrl
	adi.AddAuthority(originUrl.JoinPath("book0"))

	return WriteStates(db, adi)
}

func CreateAdiWithCredits(db DB, key tmed25519.PrivKey, urlStr types.String, credits float64) error {
	err := CreateADI(db, key, urlStr)
	if err != nil {
		return err
	}

	u, err := url.Parse(string(urlStr))
	if err != nil {
		return err
	}

	return AddCredits(db, u.JoinPath("book0/1"), credits)
}

func CreateTokenAccount(db DB, accUrl, tokenUrl string, tokens float64, lite bool) error {
	u, err := url.Parse(accUrl)
	if err != nil {
		return err
	}

	tu, err := url.Parse(tokenUrl)
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
	u, err := url.Parse(urlStr)
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

func CreateKeyPage(db DB, bookUrlStr types.String, keys ...tmed25519.PubKey) error {
	bookUrl, err := url.Parse(*bookUrlStr.AsString())
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
	page.Keys = make([]*protocol.KeySpec, len(keys))
	for i, key := range keys {
		hash := sha256.Sum256(key.Bytes())

		page.Keys[i] = &protocol.KeySpec{
			PublicKeyHash: hash[:],
		}
	}
	book.PageCount++
	return WriteStates(db, page, book)
}

func CreateKeyBook(db DB, urlStr types.String, publicKey tmed25519.PubKey) error {
	bookUrl, err := url.Parse(*urlStr.AsString())
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
