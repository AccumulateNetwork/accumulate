package testing

import (
	"crypto/sha256"

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
	deposit.TokenUrl = types.String("dc/ACME")

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
	return CreateTokenAccount(db, string(url), "dc/ACME", tokens, true)
}

func CreateADI(db *state.StateDB, key ed25519.PrivKey, url types.String) error {
	keyHash := sha256.Sum256(key.PubKey().Bytes())

	var err error
	idState := state.NewIdentityState(url)
	idState.KeyType = state.KeyTypeSha256
	idState.KeyData = keyHash[:]
	stateObj := new(state.Object)
	stateObj.Entry, err = idState.MarshalBinary()
	if err != nil {
		return err
	}

	chainId := types.GetIdentityChainFromIdentity(url.AsString())
	return db.AddStateEntry(chainId, &types.Bytes32{}, stateObj)
}

func CreateTokenAccount(db *state.StateDB, url, tokenUrl string, tokens float64, anon bool) error {
	adi, chainPath, err := types.ParseIdentityChainPath(&url)
	if err != nil {
		return err
	}
	acctChainId := types.GetChainIdFromChainPath(&chainPath)

	acctState := state.NewTokenAccount(adi, "dc/ACME")
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

	return db.AddStateEntry(acctChainId, &types.Bytes32{}, acctObj)
}
