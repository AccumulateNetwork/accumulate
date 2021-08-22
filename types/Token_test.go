package types

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestToken(t *testing.T) {

	token := NewToken("ACME", "RoadRunner/ACME", 8)

	data, err := token.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	ti2 := Token{}

	err = ti2.UnmarshalBinary(data)
	if err != nil {
		t.Fatal(err)
	}

}

func TestTokenWithMetadata(t *testing.T) {

	token := NewToken("ACME", "RoadRunner/ACME", 8)

	metadata := json.RawMessage(fmt.Sprintf("{\"%s\":\"%s\"}", "hello", "there"))
	token.Meta = &metadata

	data, err := token.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	ti2 := Token{}

	err = ti2.UnmarshalBinary(data)
	if err != nil {
		t.Fatal(err)
	}

}
