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
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
	"gitlab.com/accumulatenetwork/accumulate/types/state"
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

func CreateFakeSyntheticDepositTx(recipient tmed25519.PrivKey) (*transactions.Envelope, error) {
	recipientAdi := AcmeLiteAddressTmPriv(recipient)

	//create a fake synthetic deposit for faucet.
	deposit := new(protocol.SyntheticDepositTokens)
	deposit.Cause = sha256.Sum256([]byte("fake txid"))
	deposit.Token = protocol.AcmeUrl()
	deposit.Amount = *new(big.Int).SetUint64(TestTokenAmount * protocol.AcmePrecision)

	tx := new(transactions.Envelope)
	tx.Transaction = new(transactions.Transaction)
	tx.Transaction.Body = deposit
	tx.Transaction.Origin = recipientAdi
	tx.Transaction.KeyPageHeight = 1

	ed := new(protocol.LegacyED25519Signature)
	tx.Transaction.Nonce = 1
	ed.PublicKey = recipient.PubKey().Bytes()
	err := ed.Sign(tx.Transaction.Nonce, recipient, tx.GetTxHash())
	if err != nil {
		return nil, err
	}

	tx.Signatures = append(tx.Signatures, ed)
	return tx, nil
}

func BuildTestTokenTxGenTx(sponsor ed25519.PrivateKey, destAddr string, amount uint64) (*transactions.Envelope, error) {
	//use the public key of the bvc to make a sponsor address (this doesn't really matter right now, but need something so Identity of the BVC is good)
	from := AcmeLiteAddressStdPriv(sponsor)

	u, err := url.Parse(destAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %v", err)
	}

	send := protocol.SendTokens{}
	send.AddRecipient(u, big.NewInt(int64(amount)))

	gtx := new(transactions.Envelope)
	gtx.Transaction = new(transactions.Transaction)
	gtx.Transaction.Body = &send
	gtx.Transaction.Origin = from

	ed := new(protocol.LegacyED25519Signature)
	gtx.Transaction.Nonce = 1
	ed.PublicKey = sponsor[32:]
	err = ed.Sign(gtx.Transaction.Nonce, sponsor, gtx.GetTxHash())
	if err != nil {
		return nil, fmt.Errorf("failed to sign TX: %v", err)
	}

	gtx.Signatures = append(gtx.Signatures, ed)

	return gtx, nil
}

func BuildTestSynthDepositGenTx() (string, ed25519.PrivateKey, *transactions.Envelope, error) {
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

	gtx := new(transactions.Envelope)
	gtx.Transaction = new(transactions.Transaction)
	gtx.Transaction.Body = deposit
	gtx.Transaction.Origin = destAddress

	ed := new(protocol.LegacyED25519Signature)
	gtx.Transaction.Nonce = 1
	ed.PublicKey = privateKey[32:]
	err := ed.Sign(gtx.Transaction.Nonce, privateKey, gtx.GetTxHash())
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to sign TX: %v", err)
	}

	gtx.Signatures = append(gtx.Signatures, ed)

	return destAddress.String(), privateKey, gtx, nil
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

	state.(protocol.CreditHolder).CreditCredits(uint64(credits * protocol.CreditPrecision))
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
	mss.Url = pageUrl
	mss.Keys = append(mss.Keys, ss)
	mss.Threshold = 1

	book := protocol.NewKeyBook()
	book.Url = bookUrl
	book.Pages = append(book.Pages, pageUrl)

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

	return AddCredits(db, u.JoinPath("page0"), credits)
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

	var chain state.Chain
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

func CreateKeyPage(db DB, urlStr types.String, keys ...tmed25519.PubKey) error {
	u, err := url.Parse(*urlStr.AsString())
	if err != nil {
		return err
	}

	mss := protocol.NewKeyPage()
	mss.Url = u
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
	group.Url = groupUrl
	states := []state.Chain{group}

	group.Pages = make([]*url.URL, len(pageUrls))
	for i, s := range pageUrls {
		u, err := url.Parse(s)
		if err != nil {
			return err
		}
		group.Pages[i] = u

		spec := new(protocol.KeyPage)
		err = db.Account(u).GetStateAs(spec)
		if err != nil {
			return err
		}

		if spec.KeyBook != nil {
			return fmt.Errorf("%q is already attached to a key book", s)
		}

		spec.KeyBook = groupUrl
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
