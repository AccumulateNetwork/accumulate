package managed

import (
	"fmt"
	"testing"

	"github.com/AccumulateNetwork/accumulated/smt/storage/database"
)

func b2i(b Hash) int {
	i := int(b[0])<<24 + int(b[1])<<16 + int(b[2])<<8 + int(b[0])
	return i
}

func i2b(i int) [32]byte {
	return [32]byte{byte(i >> 24), byte(i >> 16), byte(i >> 8), byte(i)}
}

func TestMerkleManager_GetRange(t *testing.T) {

	dbManager := new(database.Manager)
	if err := dbManager.Init("memory", ""); err != nil {
		t.Fatal(err)
	}

	MM1, err := NewMerkleManager(dbManager, 2)
	MM1.MS.InitSha256()
	if err != nil {
		t.Fatal("didn't create a Merkle Manager")
	}

	MM1.SetChainID([]byte{1})
	for i := 0; i < 1000; i++ {
		MM1.AddHash(i2b(i))
	}
	MM1.SetChainID([]byte{2})
	for i := 1000; i < 2000; i++ {
		MM1.AddHash(i2b(i))
	}

	list := MM1.GetRange([]byte{1}, -1, 3)
	if len(list) >= 3 && (b2i(list[0]) != 0 || b2i(list[1]) != 1 || b2i(list[2]) != 0) {
		t.Error(fmt.Sprintf("unexpected values %x %x %x", list[0], list[1], list[2]))
	}
}
