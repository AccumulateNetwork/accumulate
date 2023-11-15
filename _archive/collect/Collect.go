// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package collect

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// E
// Cheap throw an error because programmer is lazy
func E(err error, s string) {
	if err != nil {
		panic(s)
	}
}

// EB
// Cheap error thrown against a boolean
func EB(err bool, s string) {
	if err {
		panic(s)
	}
}

// Record
// each entry in the index for the transactions
type Record struct {
	hash  []byte
	index []byte
}

type Collect struct {
	TxCount       int               // Transactions written
	GuessCnt      int               // Just Info.  Count of guesses made to find hash
	OffsetToIndex [8]byte           // Offset to the Index Table
	Offset        int64             // Keep the offset to Index Table as a uint64 as well
	OffsetEnd     int64             // Offset end of file
	Filename      string            // Name of the output file
	TmpDirName    string            // Name of a temporary directory where we put the tmp files
	outHash       *os.File          // File holding sorted, de-duplicated hashes
	out           *os.File          // Place to collect, sort, and remove duplicates
	tmpHashFiles  map[byte]*os.File // File pointing to the output file
	tmpFiles      map[byte]*os.File // File pointing to all the temp files
}

// NewCollect
// The outputName is the full path of the desired output file.  If it exists,
// the outputName will be deleted.
//
// If build is true, then structures to build a snapshot are initialized.  If false,
// then the snapshot is simply opened to allow queries
//
// A directory of the name outputName.tmp will be created. After successful processing
// the directory will be deleted.  If one exists when NewCollect is called, that directory
// will first be deleted.
func NewCollect(outputName string, build bool) (collect *Collect, err error) {
	c := new(Collect)
	defer func() {
		if msg := recover(); msg != nil {
			collect = nil
			c.Close()
			err = fmt.Errorf("%v", msg) // Return panic's error
		}

	}()
	c.Filename = outputName
	currentDir, e1 := os.Getwd()
	E(e1, "couldn't get current directory")
	c.TmpDirName = filepath.Join(currentDir, c.Filename+".tmp")
	thd := filepath.Join(c.TmpDirName, "/hashes")

	if !build {
		return c, c.Open(outputName)
	}

	c.out, err = os.Create(c.Filename)                      // create transactions
	E(err, "create transactions fail")                      //
	_, err = c.setLength()                                  // write the offset to the index (length of all tx)
	E(err, "failed to write offset")                        //
	os.RemoveAll(c.TmpDirName)                              // Remove any lingering temp directory
	err = os.Mkdir(c.TmpDirName, os.ModePerm)               // Create a new one
	E(err, "failed to create tmp directory")                //
	err = os.Mkdir(thd, os.ModePerm)                        // Create a hashes directory
	E(err, "error creating hashes tmp directory")           //
	c.outHash, err = os.Create(filepath.Join(thd, "h.tmp")) // Create tmp hash directory
	E(err, "failed to create tmp hash file")                //

	c.tmpHashFiles = make(map[byte]*os.File, 256) // Create map for all 256 tmp hash files
	for i := 0; i < 256; i++ {                    // Create a bucket file for every byte value
		file, err := os.Create(fmt.Sprintf("%s/hash%03d", thd, i)) // Create each bucket
		E(err, "creating temp files")                              //
		c.tmpHashFiles[byte(i)] = file                             // Put in map based on index
	}

	c.tmpFiles = make(map[byte]*os.File, 256) // Create a map for all 256 tmp files
	for i := 0; i < 256; i++ {                // Create a bucket file for every byte value
		file, err := os.Create(fmt.Sprintf("%s/sort%03d", c.TmpDirName, i)) // Create each bucket
		E(err, "creating temp files")                                       //
		c.tmpFiles[byte(i)] = file                                          // Put in map based on index
	}

	return c, nil
}

// WriteHash
// Sorts Hashes into buckets.
// Allows them to be sorted, deduplicated, then written all into the hash file
func (c *Collect) WriteHash(hash []byte) (err error) {
	defer func() {
		if msg := recover(); msg != nil {
			c.Close()
			err = fmt.Errorf("%v", msg) // Return panic's error
		}

	}()
	_, err = c.outHash.Seek(0, io.SeekEnd)       // Position to the end of the file (get that index)
	E(err, "failed to sync to EOF in writeHash") //
	_, err = c.tmpHashFiles[hash[0]].Write(hash) // Write the record to the bucket indicated by the transaction hash
	E(err, "filed to write hash to tmpHashFiles")
	return nil
}

// BuildHashFile
// Collect all the hashes from the byte hash files and build one sorted Hash File
// Leaves the file pointer at the start of Hash file for easy serial access
func (c *Collect) BuildHashFile() (err error) {
	defer func() {
		if msg := recover(); msg != nil {
			c.Close()
			err = fmt.Errorf("%v", msg) // Return panic's error
		}

	}()

	var hashes [][]byte        //
	for i := 0; i < 256; i++ { //                                  Run through all buckets
		hashes = hashes[:0]
		_, err := c.tmpHashFiles[byte(i)].Seek(0, io.SeekStart)      // Seek to start of tmp file
		E(err, "failed set 0 on tmp hash file")                      //
		info, e1 := os.Stat(c.tmpHashFiles[byte(i)].Name())          // Make sure bucket isn't hilariously too big
		E(e1, "failed to get file size of hash bucket")              //
		EB(info.Size() > 1024*1024*1024*2, "hash bucket is too big") // 2 GB limit on bucket, thats a 512 GB snapshot
		buffer, e2 := io.ReadAll(c.tmpHashFiles[byte(i)])            // Read the whole file in one hit
		E(e2, "failed to read a bucket")                             //
		for i := 0; i < len(buffer); i += 32 {                       // Run through the buffer and break up the hashes
			hashes = append(hashes, buffer[i:i+32])
		} //
		sort.Slice(hashes, func(i, j int) bool { //                  Sort the hashes
			return 0 > bytes.Compare(hashes[i][:], hashes[j][:])
		}) //
		var last []byte
		for _, h := range hashes { //                                Then write them all out to the end of
			if !bytes.Equal(last[:], h[:]) {
				_, e1 := c.outHash.Write(h[:])                    // every record in the
				E(e1, "failed write record hash to transactions") // index table is a hash
			}
			last = h
		} //                                                        then read the transaction.
	}
	_, err = c.outHash.Seek(0, io.SeekStart) // Leave the file position at start of file
	E(err, "failed to seek to start of file")
	return nil
}

// NumberHashes
// Returns the number of Hashes in the temporary hashes file
// Note this call is only valid after calling BuildHshFile()
func (c *Collect) NumberHashes() (hashCnt int, err error) {
	defer func() {
		if msg := recover(); msg != nil {
			c.Close()
			err = fmt.Errorf("%v", msg) // Return panic's error
		}
	}()
	info, err := c.outHash.Stat()
	E(err, "error getting information on hashes.")
	return int(info.Size()) / 32, nil
}

// GetHash
// Get the next hash out of the hash file. After all the hashes have been put into collect,
// the user then does a BuildHashFile().  Afterwards, the caller can walk serially through
// the hashes by making repeated calls to GetHash.  Returns nil on EOF
//
// GetHash will panic on any error other than EOF
func (c *Collect) GetHash() (hash []byte) {
	var h [32]byte
	l, err := c.outHash.Read(h[:])
	if errors.Is(err, io.EOF) {
		return nil
	}
	E(err, "error reading hash")
	EB(l != 32, "return from GetHash not 32 bytes")
	return h[:]
}

// Close
// Close all the files and clean up
func (c *Collect) Close() (err error) {
	defer func() {
		if msg := recover(); msg != nil {

			err = fmt.Errorf("%v", msg) // Return panic's error
		}

	}()

	// Close all the files
	err1 := c.out.Close()
	E(err1, "failed to close the output file")
	for _, f := range c.tmpHashFiles {
		err := f.Close()
		E(err, "failed to close file")
	}
	for _, f := range c.tmpFiles {
		err := f.Close()
		E(err, "failed to close file")
	}
	err2 := c.outHash.Close()

	// Remove all the tmp files; just have to kill the tmp directory
	err3 := os.RemoveAll(c.TmpDirName)
	E(err2, "failed to close outHash")
	E(err3, "failed to remove the temporary directory")
	return nil
}

// setLength
// Seek to start of file, write the length of all the transactions.
// Call to reserve the first 8 bytes of the file for the offset to the index
// Call after adding all transactions to the output file once all indexes are collected
//
// Returns the offset to the index file
func (c *Collect) setLength() (offset int64, err error) {
	defer func() {
		if msg := recover(); msg != nil {
			c.Close()
			err = fmt.Errorf("%v", msg) // Return panic's error
		}
	}()
	c.Offset, err = c.out.Seek(0, io.SeekEnd)                        // Get size of file
	E(err, "Could not get file size")                                //
	binary.BigEndian.PutUint64(c.OffsetToIndex[:], uint64(c.Offset)) // Put the length of transactions into length
	_, err = c.out.Seek(0, io.SeekStart)                             // Seek to start of transactions
	E(err, "transaction seek fail")                                  //
	_, err = c.out.Write(c.OffsetToIndex[:])                         // Write out the length
	E(err, "setLength fail")                                         //
	offset, err = c.out.Seek(0, io.SeekEnd)                          // Go back to end of transactions
	E(err, "transaction seek to end fail")
	return c.Offset, nil
}

// WriteTx
// Takes a transaction to write to c.Filename.
// Computes the hash and the length of the transaction
// Writes the length as a 4 byte value (limit 4 GB per transaction)
// Writes the transaction after the length
// Writes the index to the length to the transaction indexes
func (c *Collect) WriteTx(tx []byte) (err error) {
	defer func() {
		if msg := recover(); msg != nil {
			err = c.Close()
			err = fmt.Errorf("%v", msg) // Return panic's error
		}
	}()
	hash := sha256.Sum256(tx) // Hash of the transaction
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(tx)))

	var recordBuff [40]byte                                    // Each Index is a 32 byte hash followed by an 8 byte index
	var index int64                                            // Index of the transaction
	index, err = c.out.Seek(0, 2)                              // Position to the end of the file (get that index)
	E(err, "failed to sync to EOF in writeTx")                 //
	copy(recordBuff[:32], hash[:])                             // Copy over the hash
	binary.BigEndian.PutUint64(recordBuff[32:], uint64(index)) // Put the transaction index into the record
	_, err = c.tmpFiles[hash[0]].Write(recordBuff[:])          // Write the record to the bucket indicated by the transaction hash
	E(err, "writing recordBuf")                                //
	_, err = c.out.Write(length[:])                            // Write the transaction length
	E(err, "failed to write length in writeTx")                //
	_, err = c.out.Write(tx)                                   // Write the transaction
	E(err, "failed to write tx in writeTx")                    //
	return nil
}

// Sort Indexes
// Now all the transactions are written to the transactions file, and all the hash/index records sorted into 256 bins.
// It would be easy enough to process each bin by the next byte and so forth, keeping our memory usage way down.
// However, one byte is likely enough for what we are doing.
func (c *Collect) SortIndexes() (err error) {
	defer func() {
		if msg := recover(); msg != nil {
			c.Close()
			err = fmt.Errorf("%v", msg) // Return panic's error
		}
	}()
	_, e := c.setLength() // Set the offset to the index table

	E(e, "failed to set length") // table at the end of the output file
	var rs []Record              //
	for i := 0; i < 256; i++ {   //         Run through all the buckets
		rs = rs[:0]
		_, err := c.tmpFiles[byte(i)].Seek(0, 0)                // Seek to start of tmp file
		E(err, "Couldn't set 0 on tmp file")                    //
		info, e1 := os.Stat(c.tmpFiles[byte(i)].Name())         // Make sure bucket isn't hilariously too big
		E(e1, "failed to get file size of bucket")              //
		EB(info.Size() > 1024*1024*1024*2, "bucket is too big") // 2 GB limit on bucket, thats a 512 GB snapshot
		buffer, e2 := io.ReadAll(c.tmpFiles[byte(i)])           // Read the whole file in one hit
		E(e2, "failed to read a bucket")                        //
		var r Record                                            //
		for i := 0; i < len(buffer); i += 40 {                  // Run through the buffer and create records
			r.hash = buffer[i : i+32]     //           Slice up the buffer into records
			r.index = buffer[i+32 : i+40] //
			rs = append(rs, r)            //           Collect 'em all
		} //
		sort.Slice(rs, func(i, j int) bool { //                      Sort them
			return 0 > bytes.Compare(rs[i].hash, rs[j].hash) //
		}) //
		for _, r := range rs { //                                    Then write them all out to the end of
			_, e1 := c.out.Write(r.hash[:])                      // every record in the
			E(e1, "couldn't write record hash to transactions")  // index table is a hash
			_, e2 := c.out.Write(r.index[:])                     // followed by an 8 byte offset
			E(e2, "couldn't write record index to transactions") //
		} //                                                        then read the transaction.
	}
	info, err := os.Stat(c.out.Name())
	E(err, "output file does not exist after SortIndexes")
	c.OffsetEnd = info.Size()

	return nil
}

// Open
// Opens the output file for read access after building a transaction file
func (c *Collect) Open(filename string) (err error) {
	defer func() {
		if msg := recover(); msg != nil {
			err = c.Close()
			err = fmt.Errorf("%v", msg) // Return panic's error
		}
	}()
	c.Filename = filename
	c.out, err = os.Open(c.Filename)
	E(err, "failed to open the transaction file")
	b, e := c.out.Read(c.OffsetToIndex[:])
	E(e, "failed to read offset to index")
	EB(b != 8, "failed to read the whole 8 bytes of the index")
	c.Offset = int64(binary.BigEndian.Uint64(c.OffsetToIndex[:]))
	info, err := os.Stat(c.out.Name())
	E(err, "output file does not exist after SortIndexes")
	c.OffsetEnd = info.Size()
	return nil
}

func (c *Collect) readTx(index int) (transaction, hash []byte, err error) {
	_, e := c.out.Seek(c.Offset+int64(index*40), 0) // Seek to the transaction entry specified
	E(e, fmt.Sprintf("index %d of transaction index table is out of bounds", index))
	var record [40]byte
	_, err = c.out.Read(record[:])
	E(err, "failed to read index record")
	txIndex := binary.BigEndian.Uint64(record[32:])
	_, err = c.out.Seek(int64(txIndex), 0)
	E(err, "failed to seek to transaction")
	var txLen [4]byte
	_, err = c.out.Read(txLen[:])
	E(err, "failed to read transaction len")
	l := int(binary.BigEndian.Uint32(txLen[:]))
	transaction = make([]byte, l)
	_, err = c.out.Read(transaction)
	E(err, "failed to read transaction")
	return transaction, record[:32], nil
}

// Fetch
// Find "KeySought" in the transactions.
// It is easy if the KeySought is an index.
// It is a bit harder if the KeySought is a hash, because we have to search
// Anything else is an error.
func (c *Collect) Fetch(KeySought interface{}) (transaction, hash []byte, err error) {
	defer func() {
		if msg := recover(); msg != nil {
			c.Close()
			err = fmt.Errorf("%v", msg) // Return panic's error
		}
	}()
	switch key := KeySought.(type) {
	case int:
		tx, hash, err := c.readTx(key)
		E(err, "index is out of bounds")
		return tx, hash, err
	case []byte:
		c.GuessCnt = 0                               // Clear the guess count (info only)
		window := int64(4)                           // How many elements we read per buffer
		step := window / 2                           // Just make step 100; optimizing is hard
		buff := make([]byte, window*40)              // Max buffer to cover window used in search
		lb := int64(0)                               // lower and upper bounds of the search for a hash
		ub := (c.OffsetEnd - c.Offset) / 40          //   that matches t.  The table is sorted by hash
		KeySoughtInt := binary.BigEndian.Uint32(key) // Get an estimate of where the key is in the list
		ratio := float64(KeySoughtInt) / 0xFFFFFFFF  // Hashes are evenly spread, so estimate location
		indexRange := float64(ub - lb)               // Range of indexes where the value might be
		guess := int64(indexRange * ratio)           // file by using the first 4 bytes of the hash sought
		_ = step
		for {
			if lb == ub { //                        If the guess is out of bounds, t wasn't found
				return nil, nil, fmt.Errorf("transaction %x... not found", key[:8])
			}
			c.GuessCnt++      //  Just count guesses in batches of roughly 10
			b := guess - step //  Will read 10 entries a round (each entry 40 bytes)
			e := guess + step //
			if b < lb {       //  Make sure b and e are both in the table
				b = lb          // If b is less than lb, then we should search at least starting at the lower bound
				e = lb + window // No worries if e > ub, because we search upward
			}
			if e > ub { //     If e > ub, then trim to upper bound
				e = ub         //   because the answer can't be past ub
				b = e - window // Beginning has to stay lower than e
				if b < lb {    // If fixing the end breaks the beginning
					b = lb //    beginning is the lb
				}
			}
			_, err = c.out.Seek(c.Offset+b*40, io.SeekStart) // c-Offset (start of table) b (entry offset) 40 (size of entries)
			E(err, "failed to seek into the index table")    //
			_, err = c.out.Read(buff[:(e-b)*40])             // Read the number of entries (max 10) at 40 bytes each
			E(err, "failed to access the index table")       //
			var hashes [][]byte
			for i := int64(0); i < e-b; i++ {
				hashes = append(hashes, buff[i*40:i*40+32])
			}
		search:
			for i := int64(0); i < e-b; i++ { // Read all the entries we read
				switch bytes.Compare(key, hashes[i]) { //
				case -1: //                                  Is key < buff+i? Gotta keep looking
					ub = b + i                //                 table hash is greater than t, a HIGHER bound
					guess = (ub*3 + lb*2) / 5 //                 Interval halve
					break search
				case 0: //                                   Is key == buff, we have found the answer!
					tx, hash, err := c.readTx(int(b + i)) // Read the tx, and return what it gives us!
					return tx, hash, err                  // Return the tx
				case 1: //                                   Is key > buff, look through the whole buff
					lb = b + i + 1            // Hash found here is lower than t a LOWER bound
					guess = (ub*2 + lb*3) / 5 //

					if i == 0 && bytes.Compare(key, hashes[e-b-1]) == 1 { // Break out of loop quick (happens a lot)
						break search
					}
				}
			}
		}
	}
	panic("type not supported by c.Fetch()")
}
