package pmt

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/smt/common"
	"go.uber.org/zap/buffer"
)

// SaveSnapShot
// Saves a snapshot of the state of the BVN/DVN.
// 1) The first thing done is copy the entire BVN and persist it to disk
// 2) Then all the states are pulled from the database and persisted.
// As long as the BVN is captured in its entirety within a block, the states
// can be persisted over other blocks (as long as we don't delete any of those states
func (b *BPT) SaveSnapShot(filename string) error {

	if b.manager == nil { //                                  Snapshot cannot be taken if we have no db
		return fmt.Errorf("No manager found for BPT") //      return error
	}

	m := NewBPTManager(b.manager.DBManager.Begin(true)) //    Get a new BPTManager with its own transaction
	bb := NewBPT(m)

	file, err := os.OpenFile(filename, os.O_CREATE|os.O_RDWR, 0600) //    Open the snapshot file
	if err != nil {
		return err
	}
	defer file.Close()

	keys, err := os.CreateTemp("", "keys") //               Assume the BPT can't fit in memory.
	if err != nil {                        //               Use a temp file to collect data
		return err
	}
	values, err := os.CreateTemp("", "values") //             Assume the BPT can't fit in memory.
	if err != nil {                            //             Use a temp file to collect data
		return err
	}
	offsets, err := os.CreateTemp("", "offsets") //            Assume the BPT can't fit in memory.
	if err != nil {                              //            Use a temp file to collect data
		return err
	}
	defer os.Remove(offsets.Name()) //                        Clean up all the temp files.
	defer os.Remove(values.Name())
	defer os.Remove(keys.Name())
	defer offsets.Close()
	defer values.Close()
	defer keys.Close()

	// Start is the first possible key in a BPT
	start, _ := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000000000")
	var place [32]byte    //                                  place tracks where we are in the BPT
	copy(place[:], start) //                                  Keys start low and go high

	file.Write(common.Uint64Bytes(bb.NumNodes)) //            Write up the number of nodes

	var cKeys, cValues, cOffsets = bytes.Buffer{}, bytes.Buffer{}, bytes.Buffer{} // caches

	// Snapshot Format
	//
	// 32 byte count of nodes (NumNodes)
	// 32 byte Key -- 32 byte offset to value [NumNodes]
	// offset[0]--> 4 byte length, value 0
	// offset[1]--> 4 byte length,value 1
	// ...
	// offset[NumNodes-1] --> 4 byte length, value NumNodes-1
	//

	valueOffset := int(bb.NumNodes)*(32+4) + 4 // 4 bytes for the length, 32 bytes + 4 byte offset for every key
	keys.Write([]byte{
		byte(bb.NumNodes >> 3),
		byte(bb.NumNodes >> 2),
		byte(bb.NumNodes >> 1),
		byte(bb.NumNodes)})

	for { //
		bptValues, next := bb.GetRange(place, 1000)
		if len(bptValues) == 0 {
			break
		}
		place = next
		for _, v := range bptValues {
			cKeys.Write(v.Key[:])
			cValues.Write(v.Hash[:])
			value, err := bb.manager.DBManager.Get(v.Hash)
			if err != nil {
				return err
			}
			cOffsets.Write([]byte{byte(len(value) >> 1), byte(len(value))})
			cOffsets.Write([]byte{
				byte(valueOffset >> 3),
				byte(valueOffset >> 2),
				byte(valueOffset >> 1),
				byte(valueOffset)})
			valueOffset += len(value) + 2
		}
		if _, err := keys.Write(cKeys.Bytes()); err != nil {
			return err
		}
		if _, err := values.Write(cValues.Bytes()); err != nil {
			return err
		}
		if _, err := offsets.Write(cOffsets.Bytes()); err != nil {
			return err
		}

	}

	if _, err = keys.Seek(0, 0); err != nil {
		return err
	}
	if _, err = offsets.Seek(0, 0); err != nil {
		return err
	}
	if _, err = values.Seek(0, 0); err != nil {
		return err
	}

	var e1,e2,e3,e4,e5 error
	var buff buffer.Buffer
	var key [32]byte
	var offset [4]byte
	var value [1024 * 128]byte
	cnt := 0
	for cnt < int(b.NumNodes) {
		buff.Reset()
		for j := 0; cnt < int(b.NumNodes) && j < 1000; j++ {
			_, e1 = keys.Read(key[:])
			_, e2 = offsets.Read(offset[:])
			_, e3 = buff.Write(key[:])
			_, e4 = buff.Write(offset[:])
			
			cnt++
		}
		_, e5 = file.Write(buff.Bytes())
		switch {
		case e1 != nil:
			return e1
		case e2 != nil:
			return e2
		case e3 != nil:
			return e3
		case e4 != nil:
			return e4
		case e5 != nil:
			return e5
		}
	}
	for {
		n,e1 := values.Read(value[:])
		if n == 0 {
			break
		}
		if e1 != nil {
			return e1
		}
		file.Write(value[:n])
	}
	return nil
}


func 