package synthetic

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"testing"
	"time"
)

func TestTokenTransactionDeposit(t *testing.T) {
	amt := types.Amount{}
	amt.SetInt64(1000)
	fromAccount := types.UrlChain("MyIdentity/MyAcmeAccount")
	toAccount := types.UrlChain("YourIdentity/MyAcmeAccount")
	tx := api.NewTokenTx(fromAccount)
	tx.AddToAccount(toAccount, &amt)

	data, err := json.Marshal(&tx)

	if err != nil {
		t.Fatal(err)
	}

	ledger := types.MarshalBinaryLedgerAdiChainPath(string(toAccount), data, time.Now().Unix())

	dep := NewTokenTransactionDeposit()

	//create a fake coinbase identity and token chain
	idchp := "fakecoinbaseid/fakecoinbasetoken"
	id := types.GetIdentityChainFromIdentity(idchp)
	cp := types.GetChainIdFromChainPath(idchp)

	txid := sha256.Sum256(ledger)
	dep.SetDeposit(txid[:], amt.AsBigInt())
	dep.SetTokenInfo(types.UrlChain(idchp))
	dep.SetSenderInfo(id[:], cp[:])
	err = dep.Valid()
	if err != nil {
		t.Fatal(err)
	}

	data, err = dep.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	dep2 := NewTokenTransactionDeposit()
	err = dep2.UnmarshalBinary(data)
	if err != nil {
		t.Fatal(err)
	}

	if dep2.DepositAmount.Cmp(&dep.DepositAmount) != 0 {
		t.Fatalf("Error marshalling deposit amount")
	}

	if bytes.Compare(dep.Txid[:], dep2.Txid[:]) != 0 {
		t.Fatalf("Error marshalling txid")
	}

	if bytes.Compare(dep.SourceAdiChain[:], dep2.SourceAdiChain[:]) != 0 {
		t.Fatalf("Error marshalling sender identity hash")
	}

	if bytes.Compare(dep.SourceChainId[:], dep2.SourceChainId[:]) != 0 {
		t.Fatalf("Error marshalling sender chain id")
	}

	if dep.TokenUrl != dep2.TokenUrl {
		t.Fatalf("Error marshalling issuer identity hash")
	}

	if dep.Metadata != nil {
		if bytes.Compare(*dep.Metadata, *dep2.Metadata) != 0 {
			t.Fatalf("Error marshalling metadata")
		}
	}

	//now test to see if we can extract only header.

	header := Header{}
	err = header.UnmarshalBinary(data)
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Compare(header.Txid[:], dep2.Txid[:]) != 0 {
		t.Fatalf("Error unmarshalling header")
	}

	//now try again in json land
	data, err = json.Marshal(dep2)
	if err != nil {
		t.Fatal(err)
	}

	//var result map[string]interface{}
	header2 := Header{}
	err = json.Unmarshal(data, &header2)
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Compare(header.Txid[:], header2.Txid[:]) != 0 {
		t.Fatalf("Error unmarshalling header from json")
	}

	if bytes.Compare(header.SourceAdiChain[:], header2.SourceAdiChain[:]) != 0 {
		t.Fatalf("Error unmarshalling header from json")
	}

	if bytes.Compare(header.SourceChainId[:], header2.SourceChainId[:]) != 0 {
		t.Fatalf("Error unmarshalling header from json")
	}

}
