package api

import (
	"bytes"
	"crypto/sha256"
	"github.com/AccumulateNetwork/accumulated/types"
	"testing"
)

func TestQuery(t *testing.T) {
	q := Query{}

	q.Url = "test/url"
	q.ChainId = types.GetChainIdFromChainPath(&q.Url).Bytes()
	q.RouteId = types.GetAddressFromIdentity(&q.Url)
	h := sha256.Sum256([]byte("test txid"))
	q.Content = h[:]

	data, err := q.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	q2 := Query{}
	err = q2.UnmarshalBinary(data)
	if err != nil {
		t.Fatal(err)
	}

	if q.Url != q2.Url {
		t.Fatal("invalid url")
	}

	if q.RouteId != q2.RouteId {
		t.Fatal("invalid route id")
	}

	if !bytes.Equal(q.Content, q2.Content) {
		t.Fatal("content not equal")
	}

	if !bytes.Equal(q.ChainId, q2.ChainId) {
		t.Fatal("chain id not equal")
	}
}
