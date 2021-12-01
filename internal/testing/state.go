package testing

import (
	"crypto/sha256"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/internal/chain"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	anon "github.com/AccumulateNetwork/accumulate/types/anonaddress"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
	"github.com/AccumulateNetwork/accumulate/types/synthetic"
	"github.com/tendermint/tendermint/crypto/ed25519"
)

type DB interface {
	LoadChainAs(chainId []byte, chain state.Chain) (*state.Object, error)
	AddStateEntry(chainId *types.Bytes32, txHash *types.Bytes32, object *state.Object)
	WriteIndex(index state.Index, chain []byte, key interface{}, value []byte)
	GetIndex(index state.Index, chain []byte, key interface{}) ([]byte, error)
}

// Token multiplier
const TokenMx = 100000000

func CreateFakeSyntheticDepositTx(sponsor, recipient ed25519.PrivKey) (*transactions.GenTransaction, error) {
	sponsorAdi := types.String(anon.GenerateAcmeAddress(sponsor.PubKey().Bytes()))
	recipientAdi := types.String(anon.GenerateAcmeAddress(recipient.PubKey().Bytes()))

	//create a fake synthetic deposit for faucet.
	fakeTxid := sha256.Sum256([]byte("fake txid"))
	// NewTokenTransactionDeposit(txId types.Bytes, from *types.String, to *types.String)
	deposit := synthetic.NewTokenTransactionDeposit(fakeTxid[:], sponsorAdi, recipientAdi)
	amtToDeposit := int64(50000)                           //deposit 50k tokens
	deposit.DepositAmount.SetInt64(amtToDeposit * TokenMx) // assume 8 decimal places
	deposit.TokenUrl = types.String(protocol.AcmeUrl().String())

	depData, err := deposit.MarshalBinary()
	if err != nil {
		return nil, err
	}

	tx := new(transactions.GenTransaction)
	tx.SigInfo = new(transactions.SignatureInfo)
	tx.Transaction = depData
	tx.SigInfo.URL = *recipientAdi.AsString()
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

func CreateAnonTokenAccount(db DB, key ed25519.PrivKey, tokens float64) error {
	url := types.String(anon.GenerateAcmeAddress(key.PubKey().Bytes()))
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

func CreateADI(db DB, key ed25519.PrivKey, urlStr types.String) error {
	keyHash := sha256.Sum256(key.PubKey().Bytes())
	identityUrl, err := url.Parse(*urlStr.AsString())
	if err != nil {
		return err
	}

	sigSpecUrl := identityUrl.JoinPath("sigspec0")
	ssgUrl := identityUrl.JoinPath("ssg0")

	ss := new(protocol.KeySpec)
	ss.PublicKey = keyHash[:]

	mss := protocol.NewSigSpec()
	mss.ChainUrl = types.String(sigSpecUrl.String())
	mss.Keys = append(mss.Keys, ss)

	ssg := protocol.NewSigSpecGroup()
	ssg.ChainUrl = types.String(ssgUrl.String()) // TODO Allow override
	ssg.SigSpecs = append(ssg.SigSpecs, types.Bytes(sigSpecUrl.ResourceChain()).AsBytes32())

	adi := state.NewADI(types.String(identityUrl.String()), state.KeyTypeSha256, keyHash[:])
	adi.SigSpecId = types.Bytes(ssgUrl.ResourceChain()).AsBytes32()

	return WriteStates(db, adi, ssg, mss)
}

func CreateTokenAccount(db DB, accUrl, tokenUrl string, tokens float64, anon bool) error {
	u, err := url.Parse(accUrl)
	if err != nil {
		return err
	}
	acctChainId := types.Bytes(u.ResourceChain()).AsBytes32()
	sigSpecId := u.Identity().JoinPath("ssg0").ResourceChain() // assume the sig spec is adi/ssg0

	var chain state.Chain
	if anon {
		account := new(protocol.AnonTokenAccount)
		account.ChainUrl = types.String(u.String())
		account.TokenUrl = tokenUrl
		account.Balance.SetInt64(int64(tokens * TokenMx))
		account.TxCount++
		chain = account
	} else {
		account := state.NewTokenAccount(u.String(), tokenUrl)
		account.SigSpecId = types.Bytes(sigSpecId).AsBytes32()
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

func CreateSigSpec(db DB, urlStr types.String, keys ...ed25519.PubKey) error {
	u, err := url.Parse(*urlStr.AsString())
	if err != nil {
		return err
	}

	mss := protocol.NewSigSpec()
	mss.ChainUrl = types.String(u.String())
	mss.Keys = make([]*protocol.KeySpec, len(keys))
	for i, key := range keys {
		mss.Keys[i] = &protocol.KeySpec{
			PublicKey: key,
		}
	}

	return WriteStates(db, mss)
}

func CreateSigSpecGroup(db DB, urlStr types.String, sigSpecUrls ...string) error {
	groupUrl, err := url.Parse(*urlStr.AsString())
	if err != nil {
		return err
	}

	group := protocol.NewSigSpecGroup()
	group.ChainUrl = types.String(groupUrl.String())
	group.SigSpecs = make([][32]byte, len(sigSpecUrls))
	states := []state.Chain{group}

	for i, s := range sigSpecUrls {
		specUrl, err := url.Parse(s)
		if err != nil {
			return err
		}

		chainId := types.Bytes(specUrl.ResourceChain()).AsBytes32()
		group.SigSpecs[i] = chainId

		spec := new(protocol.SigSpec)
		_, err = db.LoadChainAs(chainId[:], spec)
		if err != nil {
			return err
		}

		if (spec.SigSpecId != types.Bytes32{}) {
			return fmt.Errorf("%q is already attached to an SSG", s)
		}

		spec.SigSpecId = types.Bytes(groupUrl.ResourceChain()).AsBytes32()
		states = append(states, spec)
	}

	return WriteStates(db, states...)
}
