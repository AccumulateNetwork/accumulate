package multipleDB

import (
	"crypto/sha256"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/stretchr/testify/assert"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
)

const NumKeyValue = 10000000

func TestDBBlock(t *testing.T) {

	start := time.Now()
	// Create and open a new DBBlock
	dbb, err := NewDBBlock("/tmp/DBBlock", 0)
	assert.NoErrorf(t, err, "NewDBBlock failed: %v", err)
	err = dbb.Open(true)
	assert.NoErrorf(t, err, "DBBlock.Open(true) Failed: %v", err)

	// Write 1000 key values into the DBBlock
	var rh1 common.RandHash
	for i := 0; i < NumKeyValue; i++ {
		value := rh1.GetRandBuff(rh1.GetIntN(200) + 100) // Make some random data
		key := sha256.Sum256(value[:])                   // The key is the hash of the data
		dbb.Write(key, value)
	}
	err = dbb.Flush()
	assert.NoErrorf(t, err, "DBBlock.Flush() failed: %v", err)

	fmt.Printf("Time to write %v\n", time.Since(start))
	start = time.Now()

	err = dbb.Open(false)
	assert.NoErrorf(t, err, "DBBlock.Open(false) failed: %v", err)

	// Read the DBBlock and make sure it is all properly stored
	var rh2 common.RandHash // Note: generates the same sequence as rh1 does
	for i := 0; i < NumKeyValue; i++ {
		value1 := rh2.GetRandBuff(rh2.GetIntN(200) + 100) // Make some random data
		key := sha256.Sum256(value1[:])                   // The key is the hash of the data
		value2, err := dbb.Read(key)
		assert.NoErrorf(t, err, "DBBlock.Read(false) failed: %v", err)
		assert.EqualValues(t, value1, value2, "data retrieval error")
	}

	fmt.Printf("Time to read all entries %v\n", time.Since(start))
}

func TestBaseDB(t *testing.T) {

	os.RemoveAll("tmp/DBBlock/badger")
	os.RemoveAll("tmp/DBBlock")

	start := time.Now()
	mdb := new(MDB)
	err := mdb.OpenBase("/tmp/DBBlock")
	assert.NoError(t, err, "failed to open base database")
	err = mdb.OpenDBBlock(1)
	assert.NoError(t, err, "failed to open DBBlock 0")
	

	var rh1 common.RandHash
	for i := 0; i < NumKeyValue; i++ {
		value := rh1.GetRandBuff(rh1.GetIntN(200) + 100) // Make some random data
		key := sha256.Sum256(value[:])                   // The key is the hash of the data
		err := mdb.Write(1, key, value)
		assert.NoError(t, err, "Error writing to base")
	}
	mdb.Close()

	records := humanize.Comma(NumKeyValue)
	fmt.Printf("writing base records %s took %v\n", records, time.Since(start))
}
