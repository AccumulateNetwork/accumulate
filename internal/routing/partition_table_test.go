package routing

import (
	"fmt"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"math"
	"sort"
	"strconv"
	"testing"
	"time"
)

func TestPartitions1(t *testing.T) {
	const numberOfBVNs = 256
	partitionCount := uint32(math.Pow(2, 16))

	var nodeCounters = make(map[string]uint)
	var bvnUrls = make([]*url.URL, numberOfBVNs)
	var err error
	for i := 0; i < numberOfBVNs; i++ {
		bvnUrls[i], err = url.Parse(fmt.Sprintf("acc://bvn-%d", i))
		if err != nil {
			t.Fail()
		}
	}
	var partitions = make([]*partition, partitionCount)
	for i := uint32(0); i < partitionCount; i++ {
		partitions[i] = &partition{
			partitionIdx: i,
			bvnIdx:       uint16(i % numberOfBVNs), // This would be assigned later based on available capacity of the BVNs
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
			selPartitionIdx := adiRoutingNr % uint64(partitionCount)
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
	for _, bvnUrl := range bvnNames {
		fmt.Printf("BVN URL %-7v got routed to %d times\n", bvnUrl, nodeCounters[bvnUrl])
	}
	fmt.Printf("We have %d nodes\n", len(nodeCounters))

	sort.Slice(partitions, func(i, j int) bool {
		return partitions[i].size > partitions[j].size
	})
	for i := uint32(0); i < partitionCount; i++ {
		p := partitions[i]
		if p != nil && p.size > 0 {
			fmt.Printf("Partition %d has size %d \n", p.partitionIdx, p.size)
		}
	}

	prcTime := uint64(end.UnixMilli() - start.UnixMilli())
	fmt.Printf("The processing time was %dms\n", prcTime)
	fmt.Printf("The average routing time was %.2fµs\n", float64(prcTime)/float64(total)*1000)
}
