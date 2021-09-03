package synthetic

import (
	"crypto/sha256"
	"encoding/json"
	"github.com/AccumulateNetwork/accumulated/types"
	"testing"
)

func TestNewAdiStateCreate(t *testing.T) {
	kp := types.CreateKeyPair()
	pubKeyHash := types.Bytes32(sha256.Sum256(kp.PubKey().Bytes()))
	adi := "redwagon"
	adiStateCreate := NewAdiStateCreate(adi, &pubKeyHash)

	adiStateCreate.Txid = types.Bytes32(sha256.Sum256([]byte("sometxid")))
	adiStateCreate.SourceAdiChain = types.Bytes32(sha256.Sum256([]byte("greenrock")))
	adiStateCreate.SourceChainId = types.Bytes32(sha256.Sum256([]byte("greenrock/somesource")))

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
