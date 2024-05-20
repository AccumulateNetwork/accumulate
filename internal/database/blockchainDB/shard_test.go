package blockchainDB

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShard(t *testing.T) {
	directory := filepath.Join(os.TempDir(), "ShardTest")
	os.Mkdir(directory, os.ModePerm)
	defer os.RemoveAll(directory)
	filename := filepath.Join(directory, "shard")

	shard, err := NewShard(5, filename)
	assert.NoError(t, err, err)

	entries := make(map[[32]byte][]byte)
	fr := NewFastRandom([32]byte{1, 2, 3, 4, 5})

	for i := 0; i < 100; i++ {
		if i%10 == 0 && i != 0 {
			fmt.Printf("%d ",i)
			if i%100==0 {
				fmt.Println()
			}
		}
		for i := 0; i < 100; i++ {
			entries[fr.NextHash()] = fr.RandBuff(100, 500)
		}
		for k := range entries {
			nv := fr.RandBuff(100, 500)
			shard.Put(k, nv)
			entries[k] = nv
		}
		for k, v := range entries {
			v2, err := shard.Get(k)
			assert.NoError(t, err, err)
			assert.Equal(t, v, v2, "Didn't get the right value back")
		}
	}
	shard.Close()
}
