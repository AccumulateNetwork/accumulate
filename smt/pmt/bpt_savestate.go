package pmt

import (
	"errors"
	"fmt"
	"io"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

const window = uint64(1000) //                               Process this many BPT entries at a time
const nLen = 32 + 32 + 8    //                               Each node is a key (32), hash (32), offset(8)

// SaveSnapshot
// Saves a snapshot of the state of the BVN/DVN.
// 1) The first thing done is copy the entire BVN and persist it to disk
// 2) Then all the states are pulled from the database and persisted.
// As long as the BVN is captured in its entirety within a block, the states
// can be persisted over other blocks (as long as we don't delete any of those states
//
// Snapshot Format:
//   8 byte count of nodes (NumNodes)
//   [count of nodes]node   -- each node is a Fixed length
//   [count of nodes]value  -- each value is variable length
//
// node:
//   32 byte Key    -- 8 byte offset from end of nodes to value
//   32 byte Hash   -- Hash of the value of the BPT entry
//   8  byte offset -- from end of list of nodes to start of the value
//
// value:
//   8  byte                  -- length of value
//   n  [length of value]byte -- the bytes of the value
//
func (b *BPT) SaveSnapshot(filename string, loadState func(storage.Key) ([]byte, error)) error {
	if b.manager == nil { //                                  Snapshot cannot be taken if we have no db
		return fmt.Errorf("No manager found for BPT") //      return error
	}

	file, e1 := os.OpenFile(filename, os.O_CREATE|os.O_RDWR, 0600) //    Open the snapshot file
	_, e2 := file.Write([]byte{0, 0, 0, 0, 0, 0, 0, 0})            //                    Safe Space to write number of nodes
	values, e3 := os.CreateTemp("", "values")                      //             Collect the values here

	defer file.Close()
	defer os.Remove(values.Name())
	defer values.Close()

	switch { //        Unlikely errors, but report them if found
	case e1 != nil:
		return e1
	case e2 != nil:
		return e2
	case e3 != nil:
		return e3
	}

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
		bptVals, next := b.GetRange(place, int(window)) //  Read a thousand values from the BPT
		NodeCnt += uint64(len(bptVals))
		if len(bptVals) == 0 { //                           If there are none left, we break out
			break
		}
		place = next //                                     We will get the next 1000 after the last 1000

		for _, v := range bptVals { //                      For all the key values we got (as many as 1000)
			_, e1 := file.Write(v.Key[:])                         // Write the key out
			_, e2 := file.Write(v.Hash[:])                        // Write the hash out
			_, e3 := file.Write(common.Uint64FixedBytes(vOffset)) // And the current offset to the next value

			value, e4 := loadState(v.Hash)                       // Get that next value
			vLen := uint64(len(value))                           // get the value's length as uint64
			_, e5 := values.Write(common.Uint64FixedBytes(vLen)) // Write out the length
			_, e6 := values.Write(value)                         // write out the value

			vOffset += uint64(len(value)) + 8 // then set the offest past the end of the value

			switch { //                none of these errors are likely, but if
			case e1 != nil: //         they occur, we should report at least
				return e1 //           the first one.
			case e2 != nil:
				return e2
			case e3 != nil:
				return e3
			case e4 != nil:
				return e4
			case e5 != nil:
				return e5
			case e6 != nil:
				return e6
			}
		}
	}

	_, e1 = file.Seek(0, 0)                              // Goto front of file
	_, e2 = file.Write(common.Uint64FixedBytes(NodeCnt)) //   and update number of nodes found
	_, e3 = values.Seek(0, 0)                            // Go to front of values
	_, e4 := file.Seek(0, 2)                             // Go to the end of file
	_, e5 := io.Copy(file, values)                       // Copy values to file
	switch {                                             // Not likely to fail, but report if it does
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

	return nil
}

// ReadSnapshot
//
func (b *BPT) LoadSnapshot(filename string, storeState func(storage.Key, []byte) ([32]byte, error)) error {
	if b.MaxHeight != 0 {
		return errors.New("A snapshot can only be read into a new BPT")
	}

	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	buff := make([]byte, window*(nLen))   //			          buff is a window's worth of key/hash/offset
	vBuff := make([]byte, 1024*128)       //                    Big enough to load any value. 128k?
	_, err = io.ReadFull(file, vBuff[:8]) //                    Read number of entries
	if err != nil {
		return err
	}
	numNodes, _ := common.BytesFixedUint64(vBuff) //          Convert entry count to number of BPT nodes

	fOff := uint64(8 + numNodes*(nLen)) //                    File offset to values in snapshot
	fileIndex := int64(8)               //                    Used to track progress through the file
	toRead := window                    //                    Assume a window's worth to read
	for toRead > 0 {

		if numNodes < toRead { //                             If not a window's worth of nodes left
			toRead = numNodes //                                load what is left
		}

		_, e1 := file.Seek(fileIndex, 0)               //             Go back to the current end of the node list
		_, e2 := io.ReadFull(file, buff[:toRead*nLen]) //             Read all the nodes we need for this pass
		fileIndex += int64(toRead * nLen)              //             Set the index to next set of nodes for next pass
		numNodes -= toRead                             //             Count roRead as read

		switch { //           errors not likely
		case e1 != nil: //    but report if found
			return e1
		case e2 != nil:
			return e2
		}

		for i := uint64(0); i < toRead; i++ { //                      Process the nodes we loaded
			idx := i * nLen                                           // Each node is nLen Bytes
			var key [32]byte                                          // Get the key,
			var hash [32]byte                                         //   the hash, and
			var off uint64                                            //   the value
			copy(key[:], buff[idx:idx+32])                            // Copy over the key
			copy(hash[:], buff[idx+32:idx+64])                        // Copy over the hash
			off, _ = common.BytesFixedUint64(buff[idx+64 : idx+64+8]) // And convert bytes to the offset to value

			_, e1 := file.Seek(int64(fOff+off), 0)          // Seek to the value
			_, e2 := io.ReadFull(file, vBuff[:8])           // Read length of value
			vLen, _ := common.BytesFixedUint64(vBuff)       // Convert bytes to uint64
			_, e3 := io.ReadFull(file, vBuff[:vLen])        // Read in the value
			b.Insert(key, hash)                             // Insert the key/hash into the BPT
			stateHash, e4 := storeState(hash, vBuff[:vLen]) // Insert the value into the DB

			switch { //        errors not likely
			case e1 != nil: // but report if found
				return e1
			case e2 != nil:
				return e2
			case e3 != nil:
				return e3
			case e4 != nil:
				return e4
			}

			if hash != stateHash {
				return fmt.Errorf("hash does not match for key %X", key)
			}
		}
	}
	return nil
}
