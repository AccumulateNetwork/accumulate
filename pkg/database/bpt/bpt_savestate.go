// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"crypto/sha256"
	"io"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// SaveSnapshotV1
// Saves a snapshot of the state of the BVN/DVN.
// 1) The first thing done is copy the entire BVN and persist it to disk
// 2) Then all the states are pulled from the database and persisted.
// As long as the BVN is captured in its entirety within a block, the states
// can be persisted over other blocks (as long as we don't delete any of those states
//
// Snapshot Format:
//
//	8 byte count of nodes (NumNodes)
//	[count of nodes]node   -- each node is a Fixed length
//	[count of nodes]value  -- each value is variable length
//
// node:
//
//	32 byte Key    -- 8 byte offset from end of nodes to value
//	32 byte Hash   -- Hash of the value of the BPT entry
//	8  byte offset -- from end of list of nodes to start of the value
//
// value:
//
//	8  byte                  -- length of value
//	n  [length of value]byte -- the bytes of the value
func SaveSnapshotV1(b *BPT, file io.WriteSeeker, loadState func(key storage.Key, hash [32]byte) ([]byte, error)) error {
	err := b.executePending()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	_, e1 := file.Write([]byte{0, 0, 0, 0, 0, 0, 0, 0}) //                    Safe Space to write number of nodes
	values, e2 := os.CreateTemp("", "values")           //             Collect the values here

	defer os.Remove(values.Name())
	defer values.Close()

	switch { //        Unlikely errors, but report them if found
	case e1 != nil:
		return e1
	case e2 != nil:
		return e2
	}

	it := b.Iterate(1000) // Process 100 entries at a time
	vOffset := uint64(0)  // Offset from the beginning of value section
	NodeCnt := uint64(0)  // Recalculate number of nodes
	for it.Next() {       //
		bptVals := it.Value() // Read a thousand values from the BPT (intentionally mask the other variable)
		NodeCnt += uint64(len(bptVals))

		s := b.mustLoadState()
		for _, v := range bptVals { //                      For all the key values we got (as many as 1000)
			kh := v.Key.Hash()
			var vh [32]byte
			if s.ArbitraryValues {
				vh = sha256.Sum256(v.Value)
			} else {
				vh = [32]byte(v.Value)
			}

			_, e1 := file.Write(kh[:])                            // Write the key out
			_, e2 := file.Write(vh[:])                            // Write the hash out
			_, e3 := file.Write(common.Uint64FixedBytes(vOffset)) // And the current offset to the next value

			value, e4 := loadState(kh, vh)                       // Get that next value
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

	_, e1 = file.Seek(0, io.SeekStart)                   // Goto front of file
	_, e2 = file.Write(common.Uint64FixedBytes(NodeCnt)) //   and update number of nodes found
	_, e3 := values.Seek(0, io.SeekStart)                // Go to front of values
	_, e4 := file.Seek(0, io.SeekEnd)                    // Go to the end of file
	_, e5 := io.Copy(file, values)                       // Copy values to file
	switch {                                             // Not likely to fail, but report if it does
	case it.Err() != nil:
		return it.Err()
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

// LoadSnapshotV1 restores a snapshot to the BPT
func LoadSnapshotV1(b *BPT, file ioutil2.SectionReader, storeState func(key storage.Key, hash [32]byte, reader ioutil2.SectionReader) error) error {
	s, err := b.loadState()
	if err != nil {
		return err
	}
	if s.MaxHeight != 0 || len(b.pending) > 0 {
		return errors.BadRequest.With("a snapshot can only be read into a new BPT")
	}

	return ReadSnapshotV1(file, func(key storage.Key, hash [32]byte, reader ioutil2.SectionReader) error {
		err := b.Insert(record.KeyFromHash(key), hash[:]) // Insert the key/hash into the BPT
		if err != nil {
			return err
		}
		return storeState(key, hash, reader) // Insert the value into the DB
	})
}

// ReadSnapshotV1 reads a snapshot
func ReadSnapshotV1(file ioutil2.SectionReader, storeState func(key storage.Key, hash [32]byte, reader ioutil2.SectionReader) error) error {
	const window = 1000                    //                    Process this many BPT entries at a time
	const nLen = 32 + 32 + 8               //                    Each node is a key (32), hash (32), offset(8)
	buff := make([]byte, window*(nLen))    //			         buff is a window's worth of key/hash/offset
	vBuff := make([]byte, 1024*128)        //                    Big enough to load any value. 128k?
	_, err := io.ReadFull(file, vBuff[:8]) //                    Read number of entries
	if err != nil {
		return err
	}
	numNodes, _ := common.BytesFixedUint64(vBuff) //          Convert entry count to number of BPT nodes

	fOff := 8 + numNodes*(nLen) //                    File offset to values in snapshot
	fileIndex := int64(8)       //                    Used to track progress through the file
	toRead := uint64(window)    //                    Assume a window's worth to read
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

			_, e1 := file.Seek(int64(fOff+off), 0)                               // Seek to the value
			_, e2 := io.ReadFull(file, vBuff[:8])                                // Read length of value
			vLen, _ := common.BytesFixedUint64(vBuff)                            // Convert bytes to uint64
			section := io.NewSectionReader(file, int64(fOff+off+8), int64(vLen)) // Create a section reader
			e3 := storeState(key, hash, section)                                 // Insert the value into the DB

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
