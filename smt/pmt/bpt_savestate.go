package pmt

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/internal/testing/e2e"
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
	file.Write(common.Uint64FixedBytes(b.NumNodes)) //       Write up the number of nodes

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

	vOffset := 8 + b.NumNodes*(32+8) //            8 bytes for the length, 32 bytes + 4 byte offset for every key

	for { //
		bptValues, next := b.GetRange(place, window) //     Read a thousand values from the BPT
		if len(bptValues) == 0 {                     //     If there are none left, we break out
			break
		}
		place = next //                                     We will get the next 1000 after the last 1000

		for _, v := range bptValues { //                    For all the key values we got (as many as 1000)
			file.Write(v.Key[:])                         // Write the key out
			file.Write(common.Uint64FixedBytes(vOffset)) // And the current offset to the next value

			value, e1 := b.manager.DBManager.Get(v.Hash)         // Get that next value
			vLen := uint64(len(value))                           // get the value's length as uint64
			_, e2 := values.Write(common.Uint64FixedBytes(vLen)) // Write out the length
			_, e3 := values.Write(value)                         // write out the value
			vOffset += uint64(len(value)) + 8                    // then set the offest past the end of the value

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
	buff := make([]byte, 1024*128) //    A big buffer to read all the values and write them into the Snapshot
	for {
		n, e1 := values.Read(buff[:]) // Read a bunch
		if n == 0 {                   // When we read nothing, we are done
			break
		}
		if e1 != nil {
			return e1
		}
		file.Write(buff[:n]) // Write a bunch
	}
	return nil
}

// ReadSnapshot
//
func (b *BPT) ReadSnapshot(filename string) error {
	if b.NumNodes != 0 {
		return errors.New("A snapshot can only be read into a new BPT")
	}

	file, err := os.OpenFile(filename, os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	buff := make([]byte, window*(32+8))
	valueBuff := make([]byte,1024*128) // This needs to be the length of the longest state value; guessing 128k
	_, err = file.Read(buff[:8])
	if err != nil {
		return err
	}
	numNodes, _ := common.BytesFixedUint64(buff)
		
	index := uint64(0)
	for {
		n, err := file.Read(buff)
		if err != nil {
			return err
		}

		for n > 0 && index < window*(32+8) && index < numNodes {
			hash := buff[:32]
			offset,_ := common.BytesFixedUint64(buff[32:])
			_, e1 := file.Seek(0,int(offset))
			_, e2 := file.Read(valueBuff[:8])
			valueLen,_ := common.BytesFixedUint64(valueBuff)
			_, e3 := file.Read(valueBuff[:valueLen])
			hash2 := sha256.Sum256(valueBuff[:valueLen])
			if !bytes.Equal(hash,hash2[:]){
				return fmt.Errorf("Hash in snapshot does not match hash of data %x %x",hash,hash2)
			}
			switch {
			case e1 != nil :
				return e1
			case e2 != nil :
				return e2
			case e3 != nil :
				return e3
			}
			index += 32+8
			n -= 32+8
		}
		if index >= numNodes{
			break
		}
	}
	return nil
}
