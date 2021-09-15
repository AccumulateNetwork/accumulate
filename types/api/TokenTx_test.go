package api

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/AccumulateNetwork/accumulated/types"
)

func TestTokenTransaction(t *testing.T) {
	tt := NewTokenTx("WileECoyote/MyACMETokens")

	toAmt := types.Amount{}
	toAmt.SetInt64(6500)
	tt.AddToAccount("AcmeCorporation/ACMETokens", &toAmt)

	toAmt.SetInt64(2500)
	tt.AddToAccount("RoadRunner/beep/beep", &toAmt)

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
		if v.URL != tt2.To[i].URL {
			t.Fatalf("to url doesn't match for %d", i)
		}
		if bytes.Compare(v.Amount.Bytes(), tt2.To[i].Amount.Bytes()) != 0 {
			t.Fatalf("amount doesn't match for %d", i)
		}
	}
}
