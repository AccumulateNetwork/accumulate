package db

import (
	"encoding/binary"
	"testing"
)

func databaseTests(t *testing.T, db DB) {

	//write some data
	for i := 0; i < 1000; i++ {
		var bucket [4]byte
		var key [4]byte
		var value [4]byte
		binary.LittleEndian.PutUint32(bucket[:], uint32(i/100))
		binary.LittleEndian.PutUint32(key[:], uint32(i))
		binary.LittleEndian.PutUint32(value[:], uint32(1000-i))
		err := db.Put(bucket[:], key[:], value[:])
		if err != nil {
			t.Fatal(err)
		}
	}

	//read back the data
	for i := uint32(0); i < 1000; i++ {
		var bucket [4]byte
		var key [4]byte
		binary.LittleEndian.PutUint32(bucket[:], i/100)
		binary.LittleEndian.PutUint32(key[:], i)

		value, err := db.Get(bucket[:], key[:])
		if err != nil {
			t.Fatal(err)
		}
		//the value is 1000-key
		if binary.LittleEndian.Uint32(value) != 1000-i {
			t.Error("Did not read data properly")
		}
	}

	//read back by bucket
	for i := uint32(0); i < 10; i++ {
		var bucket [4]byte
		binary.LittleEndian.PutUint32(bucket[:], i)
		buck, err := db.GetBucket(bucket[:])
		if err != nil {
			t.Fatal(err)
		}
		if len(buck.KeyValueList) != 100 {
			t.Fatalf("invalid number of key/value pairs in bucket, expected %v, got %v", 100, len(buck.KeyValueList))
		}
		for _, kv := range buck.KeyValueList {
			//make sure the key belongs in the bucket
			key := binary.LittleEndian.Uint32(kv.Key)
			if i != key/100 {
				t.Fatal("key doesn't belong in the bucket")
			}
			//now check the value
			value := binary.LittleEndian.Uint32(kv.Value)
			//value is 1000-key
			if value != 1000-key {
				t.Fatal("value read back from bucket doesn't match expected")
			}
		}
	}
}
