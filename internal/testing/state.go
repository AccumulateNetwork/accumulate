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

func CreateAnonTokenAccount(db *state.StateDB, accountKey ed25519.PrivKey, tokens float64) error {
	accountUrl := types.String(anon.GenerateAcmeAddress(accountKey.PubKey().Bytes()))
	adi, _, err := types.ParseIdentityChainPath(accountUrl.AsString())
	if err != nil {
		return err
	}
	acctChainId := types.GetChainIdFromChainPath(&adi)

	acctState := state.NewTokenAccount(adi, "dc/ACME")
	acctState.Type = types.ChainTypeAnonTokenAccount
	acctState.Balance.SetInt64(int64(tokens * TokenMx))

	acctState.TxCount++
	acctObj := new(state.Object)
	acctObj.Entry, err = acctState.MarshalBinary()
	if err != nil {
		return err
	}

	return db.AddStateEntry(acctChainId, &types.Bytes32{}, acctObj)
}
