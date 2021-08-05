package types

import (
	"encoding/json"
	"math/big"
	"testing"
)

func TestTokenTransaction(t *testing.T) {
	tt := TokenTransaction{}
	amt := big.NewInt(10000)
	tt.SetTransferAmount(amt)

	toamt := big.NewInt(7500)
	tt.AddToAccount("RedRock/acc", toamt)

	toamt = big.NewInt(2500)
	tt.AddToAccount("RedRock/acc/sekret/subaccount", toamt)

	data, err := json.Marshal(&tt)
	if err != nil {
		t.Fatalf("Error marshalling TokenTransaction %v", err)
	}
	tt2 := TokenTransaction{}
	err = json.Unmarshal(data, &tt2)
	//err = tt2.UnmarshalJSON(data)

	if err != nil {
		t.Fatalf("Error unmarshalling TokenTransaction %v", err)
	}
}
