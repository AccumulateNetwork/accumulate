package synthetic

import (
	"crypto/sha256"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"testing"

	"github.com/AccumulateNetwork/accumulated/types"
)

func TestTransactionNak(t *testing.T) {
	fromAccount := "roadrunner/acme"
	toAccount := "wileecoyote/acme"
	chainid := types.GetChainIdFromChainPath(&toAccount)

	tx := &transactions.GenTransaction{}
	tx.Routing = types.GetAddressFromIdentity(&toAccount)
	tx.ChainID = chainid[:]
	tx.SigInfo = new(transactions.SignatureInfo)
	nak := TransactionNak{}
	nak.FromUrl = types.String(fromAccount)
	nak.ToUrl = types.String(toAccount)
	nak.Txid = sha256.Sum256([]byte("a tx id"))
	nak.Code = 1
	data, err := nak.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	tx.Transaction = data

	_, err = tx.Marshal()
	if err != nil {
		t.Fatal(err)
	}

}
