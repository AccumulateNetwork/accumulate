package types

import (
	"testing"
)

func TestBase36AddressGenerationFromPubKeyHash(t *testing.T) {

	//intentionally create an empty public key for a reference address
	var pubkey [32]byte

	addr := GenerateAcmeAddress(pubkey[:])

	if addr != "acme51aszgnvnie070u9vplpzelzspw0vauvses33vgg0nnurabfegos22i7" {
		t.Fatalf("invalid address format")
	}

}
