package router

import (
	"fmt"
	"github.com/AccumulateNetwork/accumulated/blockchain/validator/types"
	"testing"
)

func TestNetworkAddress(t *testing.T) {
	m := make(map[uint64]string)
	n := make(map[uint64][]string)
	for i := range Networks {
		addr := types.GetAddressFromIdentityName(Networks[i].Name + ".accumulate")
		networkid := addr % uint64(len(Networks))
		if mms := m[networkid]; mms == "" {
			fmt.Printf("Found New ID : %d for network %s\n", networkid, Networks[i])
			m[networkid] = Networks[i].Name
			n[networkid] = append(n[networkid], Networks[i].Name)
		} else {
			fmt.Printf("Duplicate Found ID : %d for network %s\n", networkid, Networks[i])
			n[networkid] = append(n[networkid], Networks[i].Name)
		}
	}

	for i := range Networks {
		if mms := m[uint64(i)]; mms == "" {
			fmt.Printf("No Network Found for ID : %d\n", i)
		}
	}

	dupct := 0
	for i := range Networks {
		if mms := m[uint64(i)]; mms != "" {
			fmt.Printf("Network %s ID %d, Duplicates %d : ", mms, i, len(n[uint64(i)])-1)
			if len(n[uint64(i)]) == 1 {
				fmt.Printf("\n")
				continue
			}
			for v := range n[uint64(i)] {
				if v == 0 {
					continue
				}
				fmt.Printf("%s,", n[uint64(i)][v])
			}
			dupct++
			fmt.Printf("\n")
		} else {
			fmt.Printf("Network %d Unused\n", i)
		}
	}
	fmt.Printf("Total number of with Duplicates : %d\n", dupct)
	fmt.Printf("Total number of Unused Networks : %d\n", len(Networks)-len(m))
	//
	//for i := range Networks {
	//	if mms := dupct[uint64(i)]; mms != 0 {
	//		fmt.Printf("Duplicate ID %d\n",i, )
	//	}
	//}
}
