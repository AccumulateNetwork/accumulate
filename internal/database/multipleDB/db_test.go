package multipleDB

import (
	"fmt"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
)

func TestMain(t *testing.T) {
	var sdb MultipleDB
	

	numSets := 100000
	numRecords := 100
	var rk common.RandHash
	var rh common.RandHash

	start := time.Now()

	// Use a file for the data: true (<140 MB footprint)
	// Time to write 10000000 records 485.180. Records per second 20610.894
	//
	// Use a file for the data: false (<400 MB peak, <220 footprint)
	// Time to write 10000000 records 578.396. Records per second 17289.204
	sdb.UseFile = true // false 528 seconds true 474
	for i := 0; i < numSets; i++ {
		var set []keyValue
		for j := 0; j < 100; j++ {
			var kv keyValue
			kv.key = rk.NextA()
			kv.data = rh.GetRandBuff(int(rh.GetRandInt64()%1024 + 1024))
			set = append(set, kv)
		}
	}
	seconds := time.Since(start).Seconds()
	fmt.Printf("Use a file for the data: %v\n", sdb.UseFile)
	fmt.Printf("Time to write %d records %6.3f. Records per second %8.3f\n",
		numSets*numRecords, seconds, float64(numSets*numRecords)/seconds)

}
