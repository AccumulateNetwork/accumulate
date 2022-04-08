package pmt

import (
	"errors"
	"fmt"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/smt/common"
)

const window = 1000 //                                  Process this many BPT entries at a time

// SaveSnapshot
// Saves a snapshot of the state of the BVN/DVN.
// 1) The first thing done is copy the entire BVN and persist it to disk
// 2) Then all the states are pulled from the database and persisted.
// As long as the BVN is captured in its entirety within a block, the states
// can be persisted over other blocks (as long as we don't delete any of those states
//
// Snapshot Format
//
// 8 byte count of nodes (NumNodes)
// 32 byte Key -- 8 byte offset to value [NumNodes]
// offset[0]--> 8 byte length, value 0
// offset[1]--> 8 byte length, value 1
// ...
// offset[NumNodes-1] --> 8 byte length, value NumNodes-1
//
func (b *BPT) SaveSnapshot(filename string) error {
	if b.manager == nil { //                                  Snapshot cannot be taken if we have no db
		return fmt.Errorf("No manager found for BPT") //      return error
	}

	file, err := os.OpenFile(filename, os.O_CREATE|os.O_RDWR, 0600) //    Open the snapshot file
	if err != nil {
		return err
	}
	defer file.Close()
	file.Write([]byte{0, 0, 0, 0, 0, 0, 0, 0}) //                    Safe Space to write number of nodes

	values, err := os.CreateTemp("", "values") //             Collect the values here
	if err != nil {
		return err
	}
	defer os.Remove(values.Name())
	defer values.Close()

	// Start is the first possible key in a BPT
	place := [32]byte{
		255, 255, 255, 255, 255, 255, 255, 255,
		255, 255, 255, 255, 255, 255, 255, 255,
		255, 255, 255, 255, 255, 255, 255, 255,
		255, 255, 255, 255, 255, 255, 255, 255,
	}

	vOffset := uint64(0) //                                 Offset from the beginning of value section
	NodeCnt := uint64(0) //                                 Recalculate number of nodes
	for {                //
		bptValues, next := b.GetRange(place, window) //     Read a thousand values from the BPT
		NodeCnt += uint64(len(bptValues))
		if len(bptValues) == 0 { //                         If there are none left, we break out
			break
		}
		place = next //                                     We will get the next 1000 after the last 1000

		for _, v := range bptValues { //                    For all the key values we got (as many as 1000)
			file.Write(v.Key[:])                         // Write the key out
			file.Write(v.Hash[:])                        // Write the hash out
			file.Write(common.Uint64FixedBytes(vOffset)) // And the current offset to the next value

			value, e1 := b.manager.DBManager.Get(v.Hash)         // Get that next value
			vLen := uint64(len(value))                           // get the value's length as uint64
			_, e2 := values.Write(common.Uint64FixedBytes(vLen)) // Write out the length
			_, e3 := values.Write(value)                         // write out the value

			fmt.Printf("Writ k %x h %x off %x\n", v.Key[:4], v.Hash[:4], vOffset)

			vOffset += uint64(len(value)) + 8 // then set the offest past the end of the value

			switch { //                none of these errors are likely, but if
			case e1 != nil: //         they occur, we should report at least
				return e1 //           the first one.
			case e2 != nil:
				return e2
			case e3 != nil:
				return e3
			}
		}
	}

	_, e1 := file.Seek(0, 0)
	_, e2 := file.Write(common.Uint64FixedBytes(NodeCnt))

	{
		n, _ := file.Seek(0, 1)
		fmt.Printf("\n\noffset to values %d\n\n", n)
		file.Seek(0, 0)
		buff := make([]byte, 256)
		file.Read(buff)
		fmt.Printf("Len %x\n", buff[:8])
		fmt.Printf("key %x hash %x\n", buff[8:12], buff[40:44])
	}

	_, e3 := values.Seek(0, 0)
	_, e4 := file.Seek(0, 2)
	switch {
	case e1 != nil:
		return e1
	case e2 != nil:
		return e2
	case e3 != nil:
		return e3
	case e4 != nil:
		return e3
	}
	buff := make([]byte, 1024*128) //    A big buffer to read all the values and write them into the Snapshot
	for {
		n, e1 := values.Read(buff[:]) // Read a bunch
		fmt.Printf("Values: %x\n", buff[:n])
		switch {
		case n == 0: // When we read nothing, we are done
			return nil
		case e1 != nil:
			return e1
		}

		_, e2 := file.Write(buff[:n]) // Write a bunch

		if e2 != nil {
			return e2
		}
	}
}

// ReadSnapshot
//
func (b *BPT) LoadSnapshot(filename string) error {
	fmt.Println("-----------------LoadSnapshot-------------------")
	if b.MaxHeight != 0 {
		return errors.New("A snapshot can only be read into a new BPT")
	}

	file, err := os.OpenFile(filename, os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	buff := make([]byte, window*(64+8))

	file.Read(buff)
	fmt.Printf("Len %x\n", buff[:8])
	fmt.Printf("key %x hash %x\n", buff[8:12], buff[40:44])

	valueBuff := make([]byte, 1024*128) // This needs to be the length of the longest state value; guessing 128k
	file.Seek(0, 0)
	_, err = file.Read(buff[:8])
	if err != nil {
		return err
	}
	numNodes, _ := common.BytesFixedUint64(buff)
	fileOffset := uint64(8 + numNodes*(64+8))
	index := uint64(0)

	for {
		n, err := file.Read(buff)
		if err != nil {
			return err
		}

		for n > 0 && index < window*(64+8) && index < numNodes {
			var key [32]byte
			var hash [32]byte
			copy(key[:], buff[index:index+32])
			copy(hash[:], buff[index+32:index+64])
			offset, _ := common.BytesFixedUint64(buff[index+64 : index+64+8])
			_, e1 := file.Seek(int64(fileOffset+offset), 0)
			_, e2 := file.Read(valueBuff[:8])
			valueLen, _ := common.BytesFixedUint64(valueBuff)
			_, e3 := file.Read(valueBuff[fileOffset+offset : fileOffset+offset+valueLen])
			fmt.Printf("Read k %x h %x off %x val-len %x\n", key[:4], hash[:4], offset, valueBuff[:8])
			//hash2 := sha256.Sum256(valueBuff[:valueLen])
			//if hash != hash2 {
			//	return fmt.Errorf("Hashes should match %x %x", hash[:8], hash2[:8])
			//}
			b.Insert(key, hash)

			switch {
			case e1 != nil:
				return e1
			case e2 != nil:
				return e2
			case e3 != nil:
				return e3
			}
			index += 32 + 8
			n -= 32 + 8
		}
		if index >= numNodes {
			break
		}
	}
	return nil
}
