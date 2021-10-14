package testing

import (
	"crypto/sha256"

	"github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/types"
	anon "github.com/AccumulateNetwork/accumulated/types/anonaddress"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"
	ed25519 "github.com/tendermint/tendermint/crypto/ed25519"
)

// Token multiplier
const TokenMx = 100000000

func CreateFakeSyntheticDepositTx(sponsor, recipient ed25519.PrivKey) (*transactions.GenTransaction, error) {
	sponsorAdi := types.String(anon.GenerateAcmeAddress(sponsor.PubKey().Bytes()))
	recipientAdi := types.String(anon.GenerateAcmeAddress(recipient.PubKey().Bytes()))

	//create a fake synthetic deposit for faucet.
	fakeTxid := sha256.Sum256([]byte("fake txid"))
	// NewTokenTransactionDeposit(txId types.Bytes, from *types.String, to *types.String)
	deposit := synthetic.NewTokenTransactionDeposit(fakeTxid[:], &sponsorAdi, &recipientAdi)
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

func CreateAnonTokenAccount(db *state.StateDB, key ed25519.PrivKey, tokens float64) error {
	url := types.String(anon.GenerateAcmeAddress(key.PubKey().Bytes()))
	return CreateTokenAccount(db, string(url), protocol.AcmeUrl().String(), tokens, true)
}

func WriteStates(db *state.StateDB, chains ...state.Chain) error {
	for _, chain := range chains {
		b, err := chain.MarshalBinary()
		if err != nil {
			return err
		}

		u, err := url.Parse(chain.GetChainUrl())
		if err != nil {
			return err
		}

		chainId := types.Bytes(u.ResourceChain()).AsBytes32()
		db.AddStateEntry(&chainId, &types.Bytes32{}, &state.Object{Entry: b})
	}
	return nil
}

func CreateADI(db *state.StateDB, key ed25519.PrivKey, urlStr types.String) error {
	keyHash := sha256.Sum256(key.PubKey().Bytes())
	identityUrl, err := url.Parse(*urlStr.AsString())
	if err != nil {
		return err
	}

	keySetUrl := identityUrl.JoinPath("keyset0")
	keyGroupUrl := identityUrl.JoinPath("keygroup0")

	ss := new(protocol.SigSpec)
	ss.HashAlgorithm = protocol.SHA256
	ss.KeyAlgorithm = protocol.ED25519
	ss.PublicKey = keyHash[:]

	mss := protocol.NewMultiSigSpec()
	mss.ChainUrl = types.String(keySetUrl.String())
	mss.SigSpecs = append(mss.SigSpecs, ss)

	ssg := protocol.NewSigSpecGroup()
	ssg.ChainUrl = types.String(keyGroupUrl.String()) // TODO Allow override
	ssg.MultiSigSpecs = append(ssg.MultiSigSpecs, types.Bytes(keySetUrl.ResourceChain()).AsBytes32())

	adi := state.NewADI(types.String(identityUrl.String()), state.KeyTypeSha256, keyHash[:])
	adi.SigSpecId = types.Bytes(keyGroupUrl.ResourceChain()).AsBytes32()

	return WriteStates(db, adi, ssg, mss)
}

func CreateTokenAccount(db *state.StateDB, accUrl, tokenUrl string, tokens float64, anon bool) error {
	u, err := url.Parse(accUrl)
	if err != nil {
		return err
	}
	acctChainId := types.Bytes(u.ResourceChain()).AsBytes32()

	acctState := state.NewTokenAccount(u.String(), tokenUrl)
	if anon {
		acctState.Type = types.ChainTypeAnonTokenAccount
	}
	acctState.Balance.SetInt64(int64(tokens * TokenMx))

	acctState.TxCount++
	acctObj := new(state.Object)
	acctObj.Entry, err = acctState.MarshalBinary()
	if err != nil {
		return err
	}

	db.AddStateEntry(&acctChainId, &types.Bytes32{}, acctObj)
	return nil
}

func CreateKeySet(db *state.StateDB, urlStr types.String, keys ...ed25519.PubKey) error {
	u, err := url.Parse(*urlStr.AsString())
	if err != nil {
		return err
	}

	mss := protocol.NewMultiSigSpec()
	mss.ChainUrl = types.String(u.String())
	mss.SigSpecs = make([]*protocol.SigSpec, len(keys))
	for i, key := range keys {
		mss.SigSpecs[i] = &protocol.SigSpec{
			HashAlgorithm: protocol.Unhashed,
			KeyAlgorithm:  protocol.ED25519,
			PublicKey:     key,
		}
	}

	return WriteStates(db, mss)
}

func CreateKeyGroup(db *state.StateDB, urlStr types.String, keySetUrls ...string) error {
	u, err := url.Parse(*urlStr.AsString())
	if err != nil {
		return err
	}

	ssg := protocol.NewSigSpecGroup()
	ssg.ChainUrl = types.String(u.String())
	ssg.MultiSigSpecs = make([][32]byte, len(keySetUrls))
	for i, s := range keySetUrls {
		u, err := url.Parse(s)
		if err != nil {
			return err
		}
		chainId := types.Bytes(u.ResourceChain()).AsBytes32()
		ssg.MultiSigSpecs[i] = chainId
	}

	return WriteStates(db, ssg)
}
