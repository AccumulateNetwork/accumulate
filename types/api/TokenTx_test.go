package api

import (
	"bytes"
	"encoding/json"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"testing"
)

func TestTokenTransaction(t *testing.T) {
	tt := NewTokenTx("WileECoyote/MyACMETokens")

	toAmt := uint64(6500)
	tt.AddToAccount("AcmeCorporation/ACMETokens", toAmt)

	toAmt = 2500
	tt.AddToAccount("RoadRunner/beep/beep", toAmt)

	data, err := json.Marshal(&tt)
	if err != nil {
		t.Fatalf("Error marshalling TokenTransaction %v", err)
	}

	tt2 := TokenTx{}
	err = json.Unmarshal(data, &tt2)

	if err != nil {
		t.Fatalf("Error unmarshalling TokenTransaction %v", err)
	}

	data, err = tt.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	tt2 = TokenTx{}
	err = tt2.UnmarshalBinary(data)
	if err != nil {
		t.Fatal(err)
	}

	if tt.From != tt2.From {
		t.Fatalf("from doesn't match")
	}

	if len(tt.To) != len(tt2.To) {
		t.Fatalf("to length doesn't match")
	}
	for i, v := range tt.To {
		if !v.Equal(tt2.To[i]) {
			t.Fatalf("outputs do not match")
		}
	}

	ts := transactions.TokenSend{}
	ts.AccountURL = "WileECoyote/MyACMETokens"
	out := &transactions.Output{}
	out.Amount = 6500
	out.Dest = "AcmeCorporation/ACMETokens"
	ts.Outputs = append(ts.Outputs, out)
	out = &transactions.Output{}
	out.Amount = 2500
	out.Dest = "RoadRunner/beep/beep"
	toAmt = 2500
	ts.Outputs = append(ts.Outputs, out)

	data2 := ts.Marshal()
	if !bytes.Equal(data2, data) {
		t.Fatal("binary incompat")
	}
}
