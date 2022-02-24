package routing

import (
	"fmt"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"math"
	"strconv"
	"testing"
	"time"
)

type partitionCounter struct {
	adiUrlMap map[string]bool
}

func TestPartitions1(t *testing.T) {
	const numberOfBVNs = 256
	partitionCount := uint32(math.Pow(2, 16))

	var partitionCounters = make(map[uint64]partitionCounter)
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
			adiCnt:       0,
		}
	}

	total := uint64(0)
	routingTime := uint64(0)
	const loopCount = 4
	for j := 0; j < loopCount; j++ {
		for i := 0; i < (len(accWords)-1)*100; i++ {
			wordIndex := (i / 100) + 1
			accUrl := url.URL{
				Authority: strconv.Itoa(i) + "_" + accWords[wordIndex-1] + "_" + accWords[wordIndex],
				// Authority: strconv.Itoa(rand.Int()) + "_" + accWords[wordIndex-1] + "_" + accWords[wordIndex],
			}
			start := time.Now()
			adiRoutingNr := accUrl.Routing()
			selPartitionIdx := adiRoutingNr % uint64(partitionCount)
			partition := partitions[selPartitionIdx]

			// Read out BVN the URL, just include it in the time
			u := bvnUrls[partition.bvnIdx]
			if u == nil || len(u.String()) == 0 {
				t.Error("nil URL")
			}
			end := time.Now()

			if _, ok := partitionCounters[selPartitionIdx]; !ok {
				partitionCounters[selPartitionIdx] = partitionCounter{
					adiUrlMap: make(map[string]bool),
				}
			}
			partitionCounters[selPartitionIdx].adiUrlMap[accUrl.String()] = true
			partition.adiCnt = uint64(len(partitionCounters[selPartitionIdx].adiUrlMap))
			total++
			routingTime = routingTime + uint64(end.UnixMilli()-start.UnixMilli())
		}
	}

	fmt.Printf("The total number of routing iterations was %d\n", total)
	fmt.Printf("We have %d partitions in use from %d ADIs\n", len(partitionCounters), total/loopCount)

	lowest := ^uint64(0)
	highest := uint64(0)
	for i := uint32(0); i < partitionCount; i++ {
		p := partitions[i]
		if p != nil && p.adiCnt > 0 {
			if p.adiCnt > highest {
				highest = p.adiCnt
			}
			if p.adiCnt < lowest {
				lowest = p.adiCnt
			}
		}
	}
	fmt.Printf("The partition with the highest amount contains %d ADIs\n", highest)
	fmt.Printf("The partition with the lowest amount contains %d ADIs\n", lowest)
	fmt.Printf("The total routing time was %dms\n", routingTime)
	fmt.Printf("The average routing time was %.2fµs\n", float64(routingTime)/float64(total)*1000)
}
