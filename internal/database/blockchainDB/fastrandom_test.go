package blockchainDB

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUint64(t *testing.T) {
	collision1 := make(map[[32]byte]int)
	collision2 := make(map[uint64]int)
	var dist [100]float64
	var total float64

	fr := NewFastRandom([32]byte{1, 2, 3})
	for i := 0; i < 1_000_000; i++ {
		integer := fr.Uint64()
		hash := fr.NextHash()

		_, ok := collision1[hash]
		assert.Falsef(t, ok, "collision1 detected! %x", hash)
		collision1[hash] = 0

		_, ok = collision2[integer]
		assert.Falsef(t, ok, "collision2 detected! %x", hash)
		collision2[integer] = 0

		dist[fr.UintN(uint(len(dist)))]++
		total++
	}
	for i, v := range dist {
		L := float64(len(dist))
		assert.Falsef(t, v/total-1/L > .001 || v/total-1/L < -.001,
			"not evenly distributed %d,%5.3f", i, v)
	}
}

// Does a crude distribution test on characters in a random buffer. Each
// possible value (0-256) should be evenly distributed, and evenly distributed
// over every position in the random buffer.
//
// Could also test for two, three, four character sequences as well, but 
// for the purposes of this random number sequencer, these tests are good enough
func TestRandBuff(t *testing.T) {
	fr := NewFastRandom([32]byte{23, 56, 234, 123, 78, 28})
	var positionCounts [1000][256]float64
	var charCounts [256]float64
	var total = 0

	const loopCnt = 100000
	const buffLen = 1000

	for i := float64(0); i < loopCnt; i++ {
		buff := fr.RandBuff(buffLen, buffLen)
		for i, v := range buff {
			positionCounts[i][v]++
			charCounts[v]++
			total++
		}
	}
	reportCnt := 5
	expected := float64(total) / 256
	for _, v := range charCounts {
		percentErr := (expected - v) / float64(total)
		if reportCnt > 0 && (percentErr > .0001 || percentErr < -.0001) {
			assert.Falsef(t, percentErr > .0001 || percentErr < -.0001, "error char distribution %10.8f is too much", percentErr)
			reportCnt--
		}
	}
	reportCnt = 5
	for _, v := range positionCounts {
		for _, c := range v {
			percentErr := ((expected / buffLen) - c) / buffLen / float64(total)
			if reportCnt > 0 && (percentErr > .001 || percentErr < -.001) {
				reportCnt--
				fmt.Printf("%16.15f ", percentErr)
				assert.Falsef(t, percentErr > .001 || percentErr < -.001, "error in position %8.4f is too much", percentErr)
			}

		}
	}

}
