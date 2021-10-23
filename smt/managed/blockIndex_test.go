package managed

import "testing"

func TestBlockIndex_Marshal(t *testing.T) {

	const numberStructs = 1024

	var data []byte //                             Serialize numberStructs of BlockIndex into data

	bi := new(BlockIndex)                        // Use just one BlockIndex struct
	for i := uint64(0); i < numberStructs; i++ { // Serialize that struct numberStructs times
		bi.BlockIndex = i                    //    Change all the data in the struct. Set BlockIndex
		bi.MainIndex = i * 3                 //    Set MainIndex
		bi.PendingIndex = i * 2              //    Set PendingIndex (all to different values)
		data = append(data, bi.Marshal()...) //    Marshal bi into data, update data, and keep looping
	}

	for i := uint64(0); i < numberStructs; i++ { // Now validate them by repeatedly unmarshalling bi
		data = bi.UnMarshal(data) //               Unmarshal struct
		if bi.BlockIndex != i {   //               Sort that it is the same as what we set (use same scalar on i)
			t.Fatalf("error with the BlockIndex value %d", i)
		}
		if bi.MainIndex != i*3 { //                Sort that it is the same as what we set (use same scalar on i)
			t.Fatalf("error with the ElementIndex Value %d", i*3)
		}
		if bi.PendingIndex != i*2 { //             Sort that it is the same as what we set (use same scalar on i)
			t.Fatalf("error with the ElementIndex Value %d", i*3)
		}
	}

}
