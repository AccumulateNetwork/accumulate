package blockchainDB

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var fr = NewFastRandom([32]byte{1, 2, 3, 4, 5, 6})

func Test_dBBkey(t *testing.T) {
	dbbKey := new(DBBKey)
	dbbKey.Length = 7
	dbbKey.Offset = 101

	k := fr.NextHash()
	b := dbbKey.Bytes(k)
	nk, dbbKey2, err := GetDBBKey(b)
	assert.NoError(t, err, "should un-marshal")
	assert.Equal(t, k, nk, "Keys should be the same")
	assert.Equal(t, dbbKey2.Offset, dbbKey.Offset, "Offset should be the same")
	assert.Equal(t, dbbKey2.Length, dbbKey.Length, "Length should be the same")
}
