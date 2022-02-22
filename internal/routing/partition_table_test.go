package routing

import (
	"fmt"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"testing"
	"time"
)

func TestPartitions1(t *testing.T) {
	var nodeCounters = make(map[string]uint)
	var bvnUrls = make([]*url.URL, 256)
	var err error
	for i := 0; i < 256; i++ {
		bvnUrls[i], err = url.Parse(fmt.Sprintf("acc://bvn-%d", i))
		if err != nil {
			t.Fail()
		}
	}
	dimension := uint32(math.Pow(2, 24)) // 16777216
	var partitions = make([]*partition, dimension)
	for i := uint32(0); i < dimension; i++ {
		partitions[i] = &partition{
			partitionIdx: uint32(i),
			bvnIdx:       uint16(rand.Uint32() & 0x00FF),
			size:         0,
		}
	}

	total := uint64(0)
	start := time.Now()
	for j := 0; j < 100; j++ {
		for i := 0; i < len(accWords); i += 2 {
			accUrl := url.URL{
				Authority: strconv.Itoa(i) + "_" + accWords[i] + "_" + accWords[i+1],
			}
			adiRoutingNr := accUrl.Routing()
			selPartitionIdx := adiRoutingNr % uint64(dimension)
			partition := partitions[selPartitionIdx]

			u := bvnUrls[partition.bvnIdx]
			if u == nil {
				t.Error("nil URL")
			}
			var selectedBvn = u.String()
			if _, ok := nodeCounters[selectedBvn]; !ok {
				nodeCounters[selectedBvn] = 0
			}
			nodeCounters[selectedBvn]++
			partition.size++
			total++
		}
	}
	end := time.Now()

	fmt.Printf("The total number of lookups is %d\n", total)

	bvnNames := make([]string, 0, len(nodeCounters))
	for name := range nodeCounters {
		bvnNames = append(bvnNames, name)
	}
	sort.Slice(bvnNames, func(i, j int) bool {
		return nodeCounters[bvnNames[i]] > nodeCounters[bvnNames[j]]
	})
	for _, name := range bvnNames {
		fmt.Printf("%-7v %v\n", name, nodeCounters[name])
	}
	fmt.Printf("We have %d nodes\n", len(nodeCounters))

	sort.Slice(partitions, func(i, j int) bool {
		return partitions[i].size > partitions[j].size
	})
	for i := uint32(0); i < dimension; i++ {
		p := partitions[i]
		if p != nil && p.size > 0 {
			fmt.Printf("Partition %d has size %d \n", p.partitionIdx, p.size)
		}
	}

	fmt.Printf("The processing time was %dms\n", end.UnixMilli()-start.UnixMilli())
}
