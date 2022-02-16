package routing

import (
	"fmt"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"math/rand"
	"sort"
	"strconv"
	"testing"
)

func TestPartitions1(t *testing.T) {
	var counters = make(map[string]uint)
	var bvnUrls = make([]*url.URL, 256)
	var err error
	for i := uint8(0); i < 255; i++ {
		bvnUrls[i], err = url.Parse(fmt.Sprintf("acc://bvn-%d", i))
		if err != nil {
			t.Fail()
		}
	}
	var partitions = make([]*partition, 65536)
	for i := uint16(0); i < 65535; i++ {
		partitions[i] = &partition{
			partitionIdx: i,
			bvnIdx:       uint16(rand.Uint32() & 0x00FF),
			size:         rand.Uint64(),
		}
	}

	for j := 0; j < 100; j++ {
		for i := 0; i < len(accWords); i += 2 {
			accUrl := url.URL{
				Authority: strconv.Itoa(i) + "_" + accWords[i] + "_" + accWords[i+1],
			}
			adiRoutingNr := accUrl.Routing()
			selPartitionIdx := adiRoutingNr % uint64(65536)
			partition := partitions[selPartitionIdx]

			u := bvnUrls[partition.bvnIdx]
			t.Logf("selected idx %d, bvn %s", partition.bvnIdx, u.Hostname())
			var selectedBvn = u.String()
			if _, ok := counters[selectedBvn]; !ok {
				counters[selectedBvn] = 0
			}
			counters[selectedBvn]++
		}
	}

	bvnNames := make([]string, 0, len(counters))
	for name := range counters {
		bvnNames = append(bvnNames, name)
	}
	sort.Slice(bvnNames, func(i, j int) bool {
		return counters[bvnNames[i]] > counters[bvnNames[j]]
	})
	for _, name := range bvnNames {
		fmt.Printf("%-7v %v\n", name, counters[name])
	}
	fmt.Printf("We have %d shards\n", len(counters))
}
