package managed

import (
	"testing"

	"github.com/AccumulateNetwork/accumulated/smt/storage/database"
)

func b2i(b Hash) int64 {
	i := int64(b[0])<<24 + int64(b[1])<<16 + int64(b[2])<<8 + int64(b[3])
	return i
}

func i2b(i int64) [32]byte {
	return [32]byte{byte(i >> 24), byte(i >> 16), byte(i >> 8), byte(i)}
}

func TestConversions(t *testing.T) {
	for i := int64(0); i < 10000; i++ {
		if i != b2i(i2b(i)) {
			t.Fatalf("failed %d", i)
		}

	}
}

func TestMerkleManager_GetRange(t *testing.T) {

	MarkPower := int64(2)
	MarkFreq := int64(4)
	NumTests := int64(37)

	dbManager := new(database.Manager)
	if err := dbManager.Init("memory", ""); err != nil {
		t.Fatal(err)
	}

	MM1, err := NewMerkleManager(dbManager, MarkPower)
	MM1.MS.InitSha256()
	if err != nil {
		t.Fatal("didn't create a Merkle Manager")
	}

	MM1.SetChainID([]byte{1})
	for i := int64(0); i < NumTests; i++ {
		MM1.AddHash(i2b(i))
	}
	/*
		MM1.SetChainID([]byte{2})
		for i := NumTests; i < NumTests*2; i++ {
			MM1.AddHash(i2b(i))
		}
	*/

	MM1.Manager.EndBatch()

	for i := int64(0); i < MarkFreq*2; i++ {
		for j := int64(0); j < NumTests+1; j++ {
			begin := j
			end := j + i
			firstIndex := j
			if j < 0 {
				firstIndex = 0
			}
			if firstIndex >= NumTests {
				firstIndex = NumTests - 1
			}
			lastIndex := end
			if j+i >= NumTests {
				lastIndex = NumTests - 1
			}
			if lastIndex < 0 {
				lastIndex = 0
			}
			list, err := MM1.GetRange([]byte{1}, begin, end)
			if begin >= 0 && begin < NumTests-1 && end > begin && end > 0 && err != nil {
				t.Fatalf("shouldn't happen %v", err)
			}
			if end > 0 && begin <= NumTests-1 {
				limit := lastIndex - firstIndex
				if limit > MarkFreq/2 {
					limit = MarkFreq / 2
				}
				if int64(len(list)) != limit {
					t.Fatalf("length of response is wrong for (%d,%d)=>(%d,%d) got %d expected %d",
						begin, end, firstIndex, lastIndex, len(list), limit)
				}
			} else {
				if len(list) != 0 {
					t.Fatalf("length of response is wrong for (%d,%d)=>(%d,%d) got %d expected 0",
						begin, end, firstIndex, lastIndex, len(list))
				}
			}

			for i, v := range list {
				if begin+int64(i) != int64(b2i(v)) {
					t.Fatalf("wrong value. Got %x=>%d range(%d-%d)[%d] expected %d",
						v[:4], b2i(v),
						begin, end,
						i,
						begin+int64(i))
				}
			}

		}
	}
}
