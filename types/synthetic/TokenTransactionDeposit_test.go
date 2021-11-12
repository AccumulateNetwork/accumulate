package synthetic

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"testing"
	"time"

	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api"
)

func TestTokenTransactionDeposit(t *testing.T) {

	amt := uint64(1000)
	fromAccount := types.String("MyIdentity/MyAcmeAccount")
	toAccount := types.String("YourIdentity/MyAcmeAccount")
	tx := api.NewTokenTx(fromAccount)
	tx.AddToAccount(toAccount, amt)

	data, err := json.Marshal(&tx)

	if err != nil {
		t.Fatal(err)
	}

	ledger := types.MarshalBinaryLedgerAdiChainPath(string(toAccount), data, time.Now().Unix())

	//create a fake coinbase identity and token chain
	idCoinbase := types.String("fakecoinbaseid/fakecoinbasetoken")

	txid := sha256.Sum256(ledger)
	dep := NewTokenTransactionDeposit(txid[:], fromAccount, toAccount)
	depAmt := types.Amount{}
	depAmt.SetInt64(int64(amt))
	err = dep.SetDeposit(idCoinbase, depAmt.AsBigInt())

	if err != nil {
		t.Fatal(err)
	}

	err = dep.Valid()
	if err != nil {
		t.Fatal(err)
	}

	data, err = dep.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	dep2 := TokenTransactionDeposit{}
	err = dep2.UnmarshalBinary(data)
	if err != nil {
		t.Fatal(err)
	}

	if dep2.DepositAmount.AsBigInt().Cmp(dep.DepositAmount.AsBigInt()) != 0 {
		t.Fatalf("Error marshalling deposit amount")
	}

	if bytes.Compare(dep.Txid[:], dep2.Txid[:]) != 0 {
		t.Fatalf("Error marshalling txid")
	}

	if dep.FromUrl[:] != dep2.FromUrl[:] {
		t.Fatalf("Error marshalling sender identity hash")
	}

	if dep.ToUrl[:] != dep2.ToUrl[:] {
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

	//now just peek at the transaction type
	if data[0] != byte(types.TxTypeSyntheticDepositTokens) {
		t.Fatal("invalid transaction type")
	}

	//now test to see if we can extract only header.
	header := Header{}

	err = header.UnmarshalBinary(data[1:])
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Compare(header.Txid[:], dep2.Txid[:]) != 0 {
		t.Fatalf("Error unmarshalling header")
	}

	//now try again in json land
	data, err = json.Marshal(&dep2)
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

	if header.FromUrl[:] != header2.FromUrl[:] {
		t.Fatalf("Error unmarshalling header from json")
	}

	if header.ToUrl[:] != header2.ToUrl[:] {
		t.Fatalf("Error unmarshalling header from json")
	}

}
