package chain_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"testing"

	. "github.com/AccumulateNetwork/accumulated/internal/chain"
	testing2 "github.com/AccumulateNetwork/accumulated/internal/testing"
	"github.com/AccumulateNetwork/accumulated/types"
	anon "github.com/AccumulateNetwork/accumulated/types/anonaddress"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

func CreateFakeAnonTokenState(adiChainPath string, key ed25519.PrivateKey) (*state.Object, []byte) {

	chainId := types.GetChainIdFromChainPath(&adiChainPath)

	so := state.Object{}
	ids := state.NewTokenAccount(adiChainPath, "dc/ACME")
	ids.Chain.Type = types.ChainTypeAnonTokenAccount

	so.Entry, _ = ids.MarshalBinary()

	//we intentionally don't set the so.StateHash & so.PrevStateHash
	return &so, chainId[:]
}

func TestAnonTokenChainSynthDepositWithAccount(t *testing.T) {
	//se := &state.StateEntry{}
	//chainId := types.Bytes(gtx.ChainID).AsBytes32()
	//se.ChainId = &chainId
	//se.AdiChain = nil
	//se.ChainState = nil
}

func TestAnonTokenTransactions(t *testing.T) {
	appId := sha256.Sum256([]byte("anon"))
	tokenUrl := types.String("dc/ACME")

	_, privKey, _ := ed25519.GenerateKey(nil)

	destAddr, destPrivKey, gtx, err := testing2.BuildTestSynthDepositGenTx(&privKey)
	_ = destAddr
	_ = destPrivKey

	if err != nil {
		t.Fatal(err)
	}
	address := anon.GenerateAcmeAddress(privKey[32:])

	//CreateFakeAnonTokenState(address)
	stateDB := &state.StateDB{}
	err = stateDB.Open("/var/tmp", appId[:], true, true)
	if err != nil {
		t.Fatal(err)
	}
	se := &state.StateEntry{}

	se.DB = stateDB

	anon := AnonToken{}

	anon.BeginBlock()

	_, err = anon.DeliverTx(se, gtx)
	if err != nil {
		t.Fatal(err)
	}

	chainId := types.Bytes(gtx.ChainID).AsBytes32()
	//try to extract the state to see if we have a valid account
	anonChain, err := stateDB.GetCurrentEntry(chainId[:])
	if err != nil {
		t.Fatal(err)
	}

	tas := state.TokenAccount{}
	err = tas.UnmarshalBinary(anonChain.Entry)
	if err != nil {
		t.Fatal(err)
	}

	if *tas.Chain.ChainUrl.AsString() != gtx.SigInfo.URL {
		t.Fatalf("invalid chain header")
	}

	if tas.Chain.Type != types.ChainTypeAnonTokenAccount {
		t.Fatalf("chain state is not an anon account, it is %s", tas.Chain.Type.Name())
	}

	if tas.TokenUrl.String != tokenUrl {
		t.Fatalf("token url of state doesn't match expected")
	}

	if tas.TxCount != 1 {
		t.Fatalf("expected a token transaction count of 1")
	}

	//now query the tx reference
	refUrl := fmt.Sprintf("%s/%d", gtx.SigInfo.URL, tas.TxCount-1)
	refChainId := types.GetChainIdFromChainPath(&refUrl)
	refChainObject, err := stateDB.GetCurrentEntry(refChainId.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	txRef := state.TxReference{}
	err = txRef.UnmarshalBinary(refChainObject.Entry)
	if err != nil {
		t.Fatal(err)
	}

	if *txRef.Chain.ChainUrl.AsString() != refUrl {
		t.Fatalf("chain header expected transaction reference")
	}
	if !bytes.Equal(txRef.TxId[:], gtx.TransactionHash()) {
		t.Fatalf("txid doesn't match")
	}

	//now move some tokens around
	gtx, err = testing2.BuildTestTokenTxGenTx(destPrivKey, address, 199)

	anon.BeginBlock()

	se.ChainId = &chainId
	se.AdiChain = &chainId
	se.ChainState = anonChain
	se.AdiState = anonChain
	se.AdiHeader = &tas.Chain
	se.ChainHeader = &tas.Chain
	_, err = anon.DeliverTx(se, gtx)
	if err != nil {
		t.Fatal(err)
	}

	//pull the chains again
	anonChain, err = stateDB.GetCurrentEntry(chainId[:])
	if err != nil {
		t.Fatal(err)
	}

	tas = state.TokenAccount{}
	err = tas.UnmarshalBinary(anonChain.Entry)
	if err != nil {
		t.Fatal(err)
	}

	if tas.TokenUrl.String != tokenUrl {
		t.Fatalf("token url of state doesn't match expected")
	}

	if tas.TxCount != 2 {
		t.Fatalf("expected a token transaction count of 2")
	}

	refUrl = fmt.Sprintf("%s/%d", gtx.SigInfo.URL, tas.TxCount-1)
	refChainId = types.GetChainIdFromChainPath(&refUrl)
	refChainObject, err = stateDB.GetCurrentEntry(refChainId.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	txRef = state.TxReference{}
	err = txRef.UnmarshalBinary(refChainObject.Entry)
	if err != nil {
		t.Fatal(err)
	}

	if *txRef.Chain.ChainUrl.AsString() != refUrl {
		t.Fatalf("chain header expected transaction reference")
	}
	if !bytes.Equal(txRef.TxId[:], gtx.TransactionHash()) {
		t.Fatalf("txid doesn't match")
	}

}

func TestAnonChain(t *testing.T) {

}
