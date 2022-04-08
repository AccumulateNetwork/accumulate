package pmt

import (
	"errors"
	"fmt"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/smt/common"
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
		bptVals, next := b.GetRange(place, int(window)) //  Read a thousand values from the BPT
		NodeCnt += uint64(len(bptVals))
		if len(bptVals) == 0 { //                           If there are none left, we break out
			break
		}
		place = next //                                     We will get the next 1000 after the last 1000

		for _, v := range bptVals { //                      For all the key values we got (as many as 1000)
			file.Write(v.Key[:])                         // Write the key out
			file.Write(v.Hash[:])                        // Write the hash out
			file.Write(common.Uint64FixedBytes(vOffset)) // And the current offset to the next value

			value, e1 := b.manager.DBManager.Get(v.Hash)         // Get that next value
			vLen := uint64(len(value))                           // get the value's length as uint64
			_, e2 := values.Write(common.Uint64FixedBytes(vLen)) // Write out the length
			_, e3 := values.Write(value)                         // write out the value

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

	_, e1 := file.Seek(0, 0)                              // Goto front of file
	_, e2 := file.Write(common.Uint64FixedBytes(NodeCnt)) //   and update number of nodes found
	_, e3 := values.Seek(0, 0)                            // Go to front of values
	_, e4 := file.Seek(0, 2)                              // Go to the end of file
	switch {                                              // Not likely to fail, but report if it does
	case e1 != nil:
		return e1
	case e2 != nil:
		return e2
	case e3 != nil:
		return e3
	case e4 != nil:
		return e3
	}

	buff := make([]byte, 1024*128) //                        Just have to copy values onto the end of file
	for {                          //                        so just blindly copy with a big buffer
		n, e1 := values.Read(buff[:]) //                     Read a bunch
		switch {                      //                     Check if done, or if an error
		case e1 == nil && n == 0: //                         When we read nothing, we are done
			return nil
		case e1 != nil:
			return e1
		}
		_, e2 := file.Write(buff[:n]) //                     Write a bunch

		if e2 != nil {
			return e2
		}
	} //                                                     rinse and repeat until all values written
}

// ReadSnapshot
//
func (b *BPT) LoadSnapshot(filename string) error {
	if b.MaxHeight != 0 {
		return errors.New("A snapshot can only be read into a new BPT")
	}

	file, err := os.OpenFile(filename, os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	buff := make([]byte, window*(nLen)) //			          buff is a window's worth of key/hash/offset
	vBuff := make([]byte, 1024*128)     //                    Big enough to load any value. 128k?
	_, err = file.Read(vBuff[:8])       //                    Read number of entries
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

		_, e1 := file.Seek(fileIndex, 0)       //             Go back to the current end of the node list
		_, e2 := file.Read(buff[:toRead*nLen]) //             Read all the nodes we need for this pass
		fileIndex += int64(toRead * nLen)      //             Set the index to next set of nodes for next pass
		numNodes -= toRead                     //             Count roRead as read

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

			_, e1 := file.Seek(int64(fOff+off), 0)      // Seek to the value
			_, e2 := file.Read(vBuff[:8])               // Read length of value
			vLen, _ := common.BytesFixedUint64(vBuff)   // Convert bytes to uint64
			_, e3 := file.Read(vBuff[:vLen])            // Read in the value
			b.Insert(key, hash)                         // Insert the key/hash into the BPT
			b.manager.DBManager.Put(hash, vBuff[:vLen]) // Insert the value into the DB

			switch { //        errors not likely
			case e1 != nil: // but report if found
				return e1
			case e2 != nil:
				return e2
			case e3 != nil:
				return e3
			}
		}
	}
	return nil
}
