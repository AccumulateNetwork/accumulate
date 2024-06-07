package blockchainDB

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
)

func TestAShard(t *testing.T) {
	shardDB, err := NewShardDB(Directory,Partition,3,3)
	assert.NoError(t,err,"failed to create a shardDB")
	shard := shardDB.Shards[0]
	fr := NewFastRandom([32]byte{1,2,3})
	for i:=0; i< 100000; i++{
		shard.Put(fr.NextHash(),fr.RandBuff(100,500))
	}
	shardDB.Close()
	shardDB, err = OpenShardDB(Directory,Partition,3)
	assert.NoError(t,err,"failed to open shardDB")
}


func TestShardDB(t *testing.T) {
    
	shardDB,err := NewShardDB(Directory, Partition,3,5)
	defer os.RemoveAll(Directory)
	
	assert.NoError(t,err,"failed to create directory")
	if err != nil {
		return
	}
	var r common.RandHash
	for i := 0; i < 3; i++ {
		key := r.NextA()
		value := r.GetRandBuff(200)
		shardDB.Put(key, value)
	}
	r = *new(common.RandHash)
	for i := 0; i < 3; i++ {
		key := r.NextA()
		value := r.GetRandBuff(200)
		v := shardDB.Get(key)
		assert.Equal(t, value, v, "did not get the same value back")
	}
}
