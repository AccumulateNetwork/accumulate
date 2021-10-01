package synthetic

import (
	"crypto/sha256"
	"encoding/json"
	"testing"

	"github.com/AccumulateNetwork/accumulated/types"
)

func TestNewAdiStateCreate(t *testing.T) {
	kp := types.CreateKeyPair()
	pubKeyHash := types.Bytes32(sha256.Sum256(kp.PubKey().Bytes()))
	fromAdi := types.String("greenrock")
	toAdi := types.String("redwagon")

	txId := types.Bytes32(sha256.Sum256([]byte("sometxid")))
	adiStateCreate := NewAdiStateCreate(txId[:], &fromAdi, &toAdi, &pubKeyHash)

	data, err := json.Marshal(adiStateCreate)
	if err != nil {
		t.Fatalf("error marshalling adiStateCreate %v", err)
	}

	adiStateCreate2 := AdiStateCreate{}
	err = json.Unmarshal(data, &adiStateCreate2)
	if err != nil {
		t.Fatalf("error unmarshalling adiStateCreate %v", err)
	}

}
