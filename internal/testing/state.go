package testing

import (
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"math/big"

	"github.com/AccumulateNetwork/accumulate/internal/chain"
	"github.com/AccumulateNetwork/accumulate/internal/database"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
	tmcrypto "github.com/tendermint/tendermint/crypto"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
)

type DB = *database.Batch

// Token multiplier
const TokenMx = protocol.AcmePrecision

func CreateFakeSyntheticDepositTx(recipient tmed25519.PrivKey) (*transactions.Envelope, error) {
	recipientAdi := AcmeLiteAddressTmPriv(recipient)

	//create a fake synthetic deposit for faucet.
	deposit := new(protocol.SyntheticDepositTokens)
	deposit.Cause = sha256.Sum256([]byte("fake txid"))
	deposit.Token = protocol.ACME
	deposit.Amount = *new(big.Int).SetUint64(5e4 * protocol.AcmePrecision)

	depData, err := deposit.MarshalBinary()
	if err != nil {
		return nil, err
	}

	tx := new(transactions.Envelope)
	tx.Transaction = new(transactions.Transaction)
	tx.Transaction.Body = depData
	tx.Transaction.Origin = recipientAdi
	tx.Transaction.KeyPageHeight = 1

	ed := new(transactions.ED25519Sig)
	tx.Transaction.Nonce = 1
	ed.PublicKey = recipient.PubKey().Bytes()
	err = ed.Sign(tx.Transaction.Nonce, recipient, tx.Transaction.Hash())
	if err != nil {
		return nil, err
	}

	tx.Signatures = append(tx.Signatures, ed)
	return tx, nil
}

func CreateLiteTokenAccount(db DB, key tmed25519.PrivKey, tokens float64) error {
	url := types.String(AcmeLiteAddressTmPriv(key).String())
	return CreateTokenAccount(db, string(url), protocol.AcmeUrl().String(), tokens, true)
}

func WriteStates(db DB, chains ...state.Chain) error {
	txid := sha256.Sum256([]byte("fake txid"))
	urls := make([]*url.URL, len(chains))
	for i, c := range chains {
		u, err := c.Header().ParseUrl()
		if err != nil {
			return err
		}
		urls[i] = u

		r := db.Account(u)
		err = r.PutState(c)
		if err != nil {
			return err
		}

		chain, err := r.Chain(protocol.MainChain, protocol.ChainTypeTransaction)
		if err != nil {
			return err
		}

		err = chain.AddEntry(txid[:])
		if err != nil {
			return err
		}
	}

	return chain.AddDirectoryEntry(func(u *url.URL, key ...interface{}) chain.Value {
		return db.Account(u).Index(key...)
	}, urls...)
}

func CreateADI(db DB, key tmed25519.PrivKey, urlStr types.String) error {
	keyHash := sha256.Sum256(key.PubKey().Bytes())
	identityUrl, err := url.Parse(*urlStr.AsString())
	if err != nil {
		return err
	}

	pageUrl := identityUrl.JoinPath("page0")
	bookUrl := identityUrl.JoinPath("book0")

	ss := new(protocol.KeySpec)
	ss.PublicKey = keyHash[:]

	mss := protocol.NewKeyPage()
	mss.ChainUrl = types.String(pageUrl.String())
	mss.Keys = append(mss.Keys, ss)
	mss.Threshold = 1

	book := protocol.NewKeyBook()
	book.ChainUrl = types.String(bookUrl.String()) // TODO Allow override
	book.Pages = append(book.Pages, pageUrl.String())

	adi := protocol.NewADI()
	adi.ChainUrl = types.String(identityUrl.String())
	adi.KeyBook = types.String(bookUrl.String())

	return WriteStates(db, adi, book, mss)
}

func CreateTokenAccount(db DB, accUrl, tokenUrl string, tokens float64, lite bool) error {
	u, err := url.Parse(accUrl)
	if err != nil {
		return err
	}

	var chain state.Chain
	if lite {
		account := new(protocol.LiteTokenAccount)
		account.ChainUrl = types.String(u.String())
		account.TokenUrl = tokenUrl
		account.Balance.SetInt64(int64(tokens * TokenMx))
		chain = account
	} else {
		account := protocol.NewTokenAccount()
		account.ChainUrl = types.String(u.String())
		account.TokenUrl = tokenUrl
		account.KeyBook = types.String(u.Identity().JoinPath("book0").String())
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
	issuer.ChainUrl = types.String(u.String())
	issuer.KeyBook = types.String(u.Identity().JoinPath("book0").String())
	issuer.Symbol = symbol
	issuer.Precision = precision

	return db.Account(u).PutState(issuer)
}

func CreateKeyPage(db DB, urlStr types.String, keys ...tmed25519.PubKey) error {
	u, err := url.Parse(*urlStr.AsString())
	if err != nil {
		return err
	}

	mss := protocol.NewKeyPage()
	mss.ChainUrl = types.String(u.String())
	mss.Threshold = 1
	mss.Keys = make([]*protocol.KeySpec, len(keys))
	for i, key := range keys {
		mss.Keys[i] = &protocol.KeySpec{
			PublicKey: key,
		}
	}

	return WriteStates(db, mss)
}

func CreateKeyBook(db DB, urlStr types.String, pageUrls ...string) error {
	groupUrl, err := url.Parse(*urlStr.AsString())
	if err != nil {
		return err
	}

	group := protocol.NewKeyBook()
	group.ChainUrl = types.String(groupUrl.String())
	group.Pages = pageUrls
	states := []state.Chain{group}

	for i, s := range pageUrls {
		specUrl, err := url.Parse(s)
		if err != nil {
			return err
		}

		group.Pages[i] = specUrl.String()

		spec := new(protocol.KeyPage)
		err = db.Account(specUrl).GetStateAs(spec)
		if err != nil {
			return err
		}

		if spec.KeyBook != "" {
			return fmt.Errorf("%q is already attached to a key book", s)
		}

		spec.KeyBook = types.String(groupUrl.String())
		states = append(states, spec)
	}

	return WriteStates(db, states...)
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

func NewWalletEntry() *transactions.WalletEntry {
	wallet := new(transactions.WalletEntry)

	wallet.Nonce = 1 // Put the private key for the origin
	_, wallet.PrivateKey, _ = ed25519.GenerateKey(nil)
	wallet.Addr = AcmeLiteAddressStdPriv(wallet.PrivateKey).String() // Generate the origin address

	return wallet
}
