package blockchainDB

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

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

	writes :=0
	reads :=0
	start := time.Now()
	for i := 0; i < 1000; i++ {
		if i%100 == 0 && i != 0 {
			fmt.Printf("Writes: %10d Reads %10d %13.0f/s \n",writes,reads,
		    float64(writes+reads)/time.Since(start).Seconds())
		}
		for i := 0; i < 10; i++ {
			entries[fr.NextHash()] = fr.RandBuff(100, 500)
		}
		for k := range entries {
			nv := fr.RandBuff(100, 500)
			writes++
			shard.Put(k, nv)
			entries[k] = nv
		}
		for k, v := range entries {
			reads++
			v2, err := shard.Get(k)
			assert.NoError(t, err, err)
			assert.Equal(t, v, v2, "Didn't get the right value back")
		}
	}
	shard.Close()
}
