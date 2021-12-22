package testing

import (
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"math/big"

	"github.com/AccumulateNetwork/accumulate/internal/chain"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
	tmcrypto "github.com/tendermint/tendermint/crypto"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
)

type DB interface {
	LoadChainAs(chainId []byte, chain state.Chain) (*state.Object, error)
	AddStateEntry(chainId *types.Bytes32, txHash *types.Bytes32, object *state.Object)
	WriteIndex(index state.Index, chain []byte, key interface{}, value []byte)
	GetIndex(index state.Index, chain []byte, key interface{}) ([]byte, error)
}

// Token multiplier
const TokenMx = protocol.AcmePrecision

func CreateFakeSyntheticDepositTx(recipient tmed25519.PrivKey) (*transactions.GenTransaction, error) {
	recipientAdi := types.String(AcmeLiteAddressTmPriv(recipient).String())

	//create a fake synthetic deposit for faucet.
	deposit := new(protocol.SyntheticDepositTokens)
	deposit.Cause = sha256.Sum256([]byte("fake txid"))
	deposit.Token = protocol.ACME
	deposit.Amount = *new(big.Int).SetUint64(5e4 * protocol.AcmePrecision)

	depData, err := deposit.MarshalBinary()
	if err != nil {
		return nil, err
	}

	tx := new(transactions.GenTransaction)
	tx.SigInfo = new(transactions.SignatureInfo)
	tx.Transaction = depData
	tx.SigInfo.URL = *recipientAdi.AsString()
	tx.SigInfo.KeyPageHeight = 1
	tx.ChainID = types.GetChainIdFromChainPath(recipientAdi.AsString())[:]
	tx.Routing = types.GetAddressFromIdentity(recipientAdi.AsString())

	ed := new(transactions.ED25519Sig)
	tx.SigInfo.Nonce = 1
	ed.PublicKey = recipient.PubKey().Bytes()
	err = ed.Sign(tx.SigInfo.Nonce, recipient, tx.TransactionHash())
	if err != nil {
		return nil, err
	}

	tx.Signature = append(tx.Signature, ed)
	return tx, nil
}

func CreateLiteTokenAccount(db DB, key tmed25519.PrivKey, tokens float64) error {
	url := types.String(AcmeLiteAddressTmPriv(key).String())
	return CreateTokenAccount(db, string(url), protocol.AcmeUrl().String(), tokens, true)
}

func WriteStates(db DB, chains ...state.Chain) error {
	for _, c := range chains {
		b, err := c.MarshalBinary()
		if err != nil {
			return err
		}

		u, err := c.Header().ParseUrl()
		if err != nil {
			return err
		}

		chainId := types.Bytes(u.ResourceChain()).AsBytes32()
		db.AddStateEntry(&chainId, &types.Bytes32{}, &state.Object{Entry: b})

		if !u.Identity().Equal(u) {
			chain.AddDirectoryEntry(db, u)
		}
	}
	return nil
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

	book := protocol.NewKeyBook()
	book.ChainUrl = types.String(bookUrl.String()) // TODO Allow override
	book.Pages = append(book.Pages, types.Bytes(pageUrl.ResourceChain()).AsBytes32())

	adi := state.NewADI(types.String(identityUrl.String()), state.KeyTypeSha256, keyHash[:])
	adi.KeyBook = types.Bytes(bookUrl.ResourceChain()).AsBytes32()

	return WriteStates(db, adi, book, mss)
}

func CreateTokenAccount(db DB, accUrl, tokenUrl string, tokens float64, lite bool) error {
	u, err := url.Parse(accUrl)
	if err != nil {
		return err
	}
	acctChainId := types.Bytes(u.ResourceChain()).AsBytes32()
	bookId := u.Identity().JoinPath("book0").ResourceChain() // assume the book is adi/book0

	var chain state.Chain
	if lite {
		account := new(protocol.LiteTokenAccount)
		account.ChainUrl = types.String(u.String())
		account.TokenUrl = tokenUrl
		account.Balance.SetInt64(int64(tokens * TokenMx))
		account.TxCount++
		chain = account
	} else {
		account := state.NewTokenAccount(u.String(), tokenUrl)
		account.KeyBook = types.Bytes(bookId).AsBytes32()
		account.Balance.SetInt64(int64(tokens * TokenMx))
		account.TxCount++
		chain = account
	}

	data, err := chain.MarshalBinary()
	if err != nil {
		return err
	}

	db.AddStateEntry(&acctChainId, &types.Bytes32{}, &state.Object{Entry: data})
	return nil
}

func CreateKeyPage(db DB, urlStr types.String, keys ...tmed25519.PubKey) error {
	u, err := url.Parse(*urlStr.AsString())
	if err != nil {
		return err
	}

	mss := protocol.NewKeyPage()
	mss.ChainUrl = types.String(u.String())
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
	group.Pages = make([][32]byte, len(pageUrls))
	states := []state.Chain{group}

	for i, s := range pageUrls {
		specUrl, err := url.Parse(s)
		if err != nil {
			return err
		}

		chainId := types.Bytes(specUrl.ResourceChain()).AsBytes32()
		group.Pages[i] = chainId

		spec := new(protocol.KeyPage)
		_, err = db.LoadChainAs(chainId[:], spec)
		if err != nil {
			return err
		}

		if (spec.KeyBook != types.Bytes32{}) {
			return fmt.Errorf("%q is already attached to a key book", s)
		}

		spec.KeyBook = types.Bytes(groupUrl.ResourceChain()).AsBytes32()
		states = append(states, spec)
	}

	return WriteStates(db, states...)
}

// AcmeLiteAddress creates an ACME lite address for the given key. FOR TESTING
// USE ONLY.
func AcmeLiteAddress(pubKey []byte) *url.URL {
	u, err := protocol.LiteAddress(pubKey, protocol.ACME)
	if err != nil {
		// LiteAddress should only return an error if the token URL is invalid,
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
