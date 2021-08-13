package synthetic

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"github.com/AccumulateNetwork/accumulated/types"
	"math/big"
	"testing"
	"time"
)

func TestTokenTransactionDeposit(t *testing.T) {
	amt := big.NewInt(1000)

	toaccount := "YourIdentity/MyAcmeAccount"
	tx := types.NewTokenTransaction(nil, nil)
	tx.SetTransferAmount(amt)
	tx.AddToAccount(toaccount, amt)

	data, err := json.Marshal(&tx)

	if err != nil {
		t.Fatal(err)
	}

	ledger := types.MarshalBinaryLedgerAdiChainPath(toaccount, data, time.Now().Unix())

	dep := NewTokenTransactionDeposit()

	//create a fake coinbase identity and token chain
	idchp := "fakecoinbaseid/fakecoinbasetoken"
	id := types.GetIdentityChainFromIdentity(idchp)
	cp := types.GetChainIdFromChainPath(idchp)

	txid := sha256.Sum256(ledger)
	dep.SetDeposit(txid[:], amt)
	dep.SetTokenInfo(id[:], cp[:])
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

	if bytes.Compare(dep.SourceIdentity[:], dep2.SourceIdentity[:]) != 0 {
		t.Fatalf("Error marshalling sender identity hash")
	}

	if bytes.Compare(dep.SourceChainId[:], dep2.SourceChainId[:]) != 0 {
		t.Fatalf("Error marshalling sender chain id")
	}

	if bytes.Compare(dep.IssuerIdentity[:], dep2.IssuerIdentity[:]) != 0 {
		t.Fatalf("Error marshalling issuer identity hash")
	}

	if bytes.Compare(dep.IssuerChainId[:], dep2.IssuerChainId[:]) != 0 {
		t.Fatalf("Error marshalling sender chain id")
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

	if bytes.Compare(header.SourceIdentity[:], header2.SourceIdentity[:]) != 0 {
		t.Fatalf("Error unmarshalling header from json")
	}

	if bytes.Compare(header.SourceChainId[:], header2.SourceChainId[:]) != 0 {
		t.Fatalf("Error unmarshalling header from json")
	}

}
