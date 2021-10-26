package query

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

func TestQuery(t *testing.T) {
	q := Query{}

	q.RouteId = 0
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

	if q.RouteId != q2.RouteId {
		t.Fatal("invalid route id")
	}

	if !bytes.Equal(q.Content, q2.Content) {
		t.Fatal("content not equal")
	}

}
