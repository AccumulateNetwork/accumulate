package managed

import "testing"

func TestBlockIndex_Marshal(t *testing.T) {
	var data []byte
	bi := new(BlockIndex)
	for i := int64(0); i < 1024; i++ {
		bi.BlockIndex = i
		bi.MainIndex = i * 3
		bi.PendingIndex = i * 2
		data = append(data, bi.Marshal()...)
	}

	for i := int64(0); i < 1024; i++ {
		data = bi.UnMarshal(data)
		if bi.BlockIndex != i {
			t.Fatalf("error with the BlockIndex value %d", i)
		}
		if bi.MainIndex != i*3 {
			t.Fatalf("error with the ElementIndex Value %d", i*3)
		}
		if bi.PendingIndex != i*2 {
			t.Fatalf("error with the ElementIndex Value %d", i*3)
		}
	}

}
