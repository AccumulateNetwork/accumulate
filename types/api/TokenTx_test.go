package api

import (
	"encoding/json"
	"github.com/AccumulateNetwork/accumulated/types"
	"testing"
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
}
