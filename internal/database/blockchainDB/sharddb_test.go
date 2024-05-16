package blockchainDB

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
)

func TestShardDB(t *testing.T) {
	var shardDB ShardDB
	var r common.RandHash
	for i := 0; i < 1000000; i++ {
		key := r.NextA()
		value := r.GetRandBuff(200)
		shardDB.Put(key, value)
	}
	r = *new(common.RandHash)
	for i := 0; i < 1000000; i++ {
		key := r.NextA()
		value := r.GetRandBuff(200)
		v := shardDB.Get(key)
		assert.Equal(t, value, v, "did not get the same value back")
	}
}

func TestShard(t *testing.T) {
	directory := filepath.Join(os.TempDir(),"ShardTest")
	os.Mkdir(directory,os.ModePerm)
	defer os.RemoveAll(directory)
	filename := filepath.Join(directory,"shard")

	shard,err := NewShard(5,filename)
	assert.NoError(t,err,err)

	entries := make(map[[32]byte][]byte)
	fr := NewFastRandom([32]byte {1,2,3,4,5})
	for i:= 0; i<100000; i++ {
		entries[fr.NextHash()] = fr.RandBuff(100,500)
	}
	for k := range entries {
		nv := fr.RandBuff(100,500)
		shard.Put(k,nv)
		entries[k]=nv
	}
	for i:=0;i<1000000,i++ {
		v2,err := shard.Get(k)
		assert.NoError(t,err,err)
		assert.Equal(t,v,v2,"Didn't get the right value back")
	}

	shard.Close()
}