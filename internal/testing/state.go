package testing

import (
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
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

// Token multiplier
const TokenMx = protocol.AcmePrecision
const TestTokenAmount = 5e5

func GenerateKey(seed ...interface{}) ed25519.PrivateKey {
	h := storage.MakeKey(seed...)
	return ed25519.NewKeyFromSeed(h[:])
}

func MustParseUrl(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

func CreateFakeSyntheticDepositTx(recipient tmed25519.PrivKey) (*protocol.Envelope, error) {
	recipientAdi := AcmeLiteAddressTmPriv(recipient)

	//create a fake synthetic deposit for faucet.
	deposit := new(protocol.SyntheticDepositTokens)
	deposit.Cause = sha256.Sum256([]byte("fake txid"))
	deposit.Token = protocol.AcmeUrl()
	deposit.Amount = *new(big.Int).SetUint64(TestTokenAmount * protocol.AcmePrecision)

	return NewTransaction().
		WithPrincipal(recipientAdi).
		WithSigner(protocol.FaucetUrl, 1).
		WithNonce(1).
		WithBody(deposit).
		Initiate(protocol.SignatureTypeLegacyED25519, recipient), nil
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
		WithNonce(1).
		WithBody(&send).
		Initiate(protocol.SignatureTypeLegacyED25519, sponsor), nil
}

func BuildTestSynthDepositGenTx() (string, ed25519.PrivateKey, *protocol.Envelope, error) {
	_, privateKey, _ := ed25519.GenerateKey(nil)
	//set destination url address
	destAddress := AcmeLiteAddressStdPriv(privateKey)

	//create a fake synthetic deposit for faucet.
	deposit := new(protocol.SyntheticDepositTokens)
	deposit.Cause = sha256.Sum256([]byte("fake txid"))
	deposit.Token = protocol.AcmeUrl()
	deposit.Amount = *new(big.Int).SetUint64(TestTokenAmount * protocol.AcmePrecision)
	// deposit := synthetic.NewTokenTransactionDeposit(txid[:], adiSponsor, destAddress)
	// amtToDeposit := int64(50000)                             //deposit 50k tokens
	// deposit.DepositAmount.SetInt64(amtToDeposit * protocol.AcmePrecision) // assume 8 decimal places
	// deposit.TokenUrl = tokenUrl

	env := NewTransaction().
		WithPrincipal(destAddress).
		WithSigner(protocol.FaucetUrl, 1).
		WithNonce(1).
		WithBody(deposit).
		Initiate(protocol.SignatureTypeLegacyED25519, privateKey)

	return destAddress.String(), privateKey, env, nil
}

func CreateLiteTokenAccount(db DB, key tmed25519.PrivKey, tokens float64) error {
	url := AcmeLiteAddressTmPriv(key).String()
	return CreateTokenAccount(db, string(url), protocol.AcmeUrl().String(), tokens, true)
}

func AddCredits(db DB, account *url.URL, acme float64) error {
	state, err := db.Account(account).GetState()
	if err != nil {
		return err
	}

	state.(protocol.CreditHolder).CreditCredits(*new(big.Int).SetUint64(uint64(acme * protocol.AcmePrecision)))
	return db.Account(account).PutState(state)
}

func CreateLiteTokenAccountWithCredits(db DB, key tmed25519.PrivKey, tokens, acme float64) error {
	url := AcmeLiteAddressTmPriv(key)
	err := CreateTokenAccount(db, url.String(), protocol.AcmeUrl().String(), tokens, true)
	if err != nil {
		return err
	}

	return AddCredits(db, url, acme)
}

func WriteStates(db DB, chains ...protocol.Account) error {
	txid := sha256.Sum256([]byte("fake txid"))
	urls := make([]*url.URL, len(chains))
	for i, c := range chains {
		urls[i] = c.Header().Url

		r := db.Account(c.Header().Url)
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
	//keyHash := sha256.Sum256(key.PubKey().Bytes()) // TODO This is not what create_identity / create_key_page do, nonce will be > 0 also
	keyHash := key.PubKey().Bytes()
	identityUrl, err := url.Parse(*urlStr.AsString())
	if err != nil {
		return err
	}

	bookUrl := identityUrl.JoinPath("book0")

	ss := new(protocol.KeySpec)
	ss.PublicKey = keyHash[:]

	mss := protocol.NewKeyPage()
	mss.Url = protocol.FormatKeyPageUrl(bookUrl, 0)
	mss.Keys = append(mss.Keys, ss)
	mss.Threshold = 1

	book := protocol.NewKeyBook()
	book.Url = bookUrl
	book.PageCount = 1

	adi := protocol.NewADI()
	adi.Url = identityUrl
	adi.KeyBook = bookUrl

	return WriteStates(db, adi, book, mss)
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

	adi := protocol.NewADI()
	adi.Url = identityUrl
	adi.KeyBook = originUrl.JoinPath("book0")

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
		account.Balance.SetInt64(int64(tokens * TokenMx))
		chain = account
	} else {
		account := protocol.NewTokenAccount()
		account.Url = u
		account.TokenUrl = tu
		account.KeyBook = u.Identity().JoinPath("book0")
		account.Balance.SetInt64(int64(tokens * TokenMx))
		chain = account
	}

	return db.Account(u).PutState(chain)
}

func CreateTokenIssuer(db DB, urlStr, symbol string, precision uint64) error {
	u, err := url.Parse(urlStr)
	if err != nil {
		return err
	}

	issuer := new(protocol.TokenIssuer)
	issuer.Url = u
	issuer.KeyBook = u.Identity().JoinPath("book0")
	issuer.Symbol = symbol
	issuer.Precision = precision

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

	page := protocol.NewKeyPage()
	page.Url = protocol.FormatKeyPageUrl(bookUrl, book.PageCount)
	page.KeyBook = bookUrl
	page.Threshold = 1
	page.Keys = make([]*protocol.KeySpec, len(keys))
	for i, key := range keys {
		page.Keys[i] = &protocol.KeySpec{
			PublicKey: key,
		}
	}
	book.PageCount++
	return WriteStates(db, page, book)
}

func CreateKeyBook(db DB, urlStr types.String, publicKeyHash ...tmed25519.PubKey) error {
	bookUrl, err := url.Parse(*urlStr.AsString())
	if err != nil {
		return err
	}

	book := protocol.NewKeyBook()
	book.Url = bookUrl
	book.PageCount = 1

	page := new(protocol.KeyPage)
	page.KeyBook = bookUrl
	page.Url = protocol.FormatKeyPageUrl(bookUrl, 0)

	if len(publicKeyHash) == 1 {
		key := new(protocol.KeySpec)
		key.PublicKey = publicKeyHash[0]
		page.Keys = []*protocol.KeySpec{key}
	} else if len(publicKeyHash) > 1 {
		return errors.New("CreateKeyBook only supports one page key at the moment") // TOOO do we need to suport this? (Also in create_book.go)
	}

	accounts := []protocol.Account{book, page}
	return WriteStates(db, accounts...)
}

// AcmeLiteAddress creates an ACME lite address for the given key. FOR TESTING
// USE ONLY.
func AcmeLiteAddress(pubKey []byte) *url.URL {
	u, err := protocol.LiteTokenAddress(pubKey, protocol.ACME)
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
