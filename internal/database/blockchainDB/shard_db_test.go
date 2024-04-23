package multipleDB

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
)

func TestShardDB(t *testing.T) {
	var shardDB ShardDB
	var r  common.RandHash
	for i:= 0; i<1000000; i++ {
		key := r.NextA()
		value := r.GetRandBuff(200)
		shardDB.Put(key,value)
	}
	r = *new(common.RandHash)
	for i:= 0; i<1000000; i++ {
		key := r.NextA()
		value := r.GetRandBuff(200)
		v := shardDB.Get(key)
		assert.Equal(t,value,v, "did not get the same value back")
	}
}