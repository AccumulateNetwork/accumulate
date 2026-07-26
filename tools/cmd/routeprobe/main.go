// routeprobe generates N random lite token accounts and routes each through the
// live routing table, reporting the partition distribution.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func main() {
	const N = 100000
	table := &protocol.RoutingTable{Routes: []protocol.Route{
		{Length: 1, Partition: "BVN1"},
		{Length: 2, Value: 2, Partition: "BVN2"},
		{Length: 2, Value: 3, Partition: "BVN3"},
	}}
	tree, err := routing.NewRouteTree(table)
	if err != nil {
		panic(err)
	}
	count := map[string]int{}
	for i := 0; i < N; i++ {
		seed := make([]byte, ed25519.SeedSize)
		rand.Read(seed)
		key := ed25519.NewKeyFromSeed(seed)
		acct, err := protocol.LiteTokenAddress(key[32:], protocol.ACME, protocol.SignatureTypeED25519)
		if err != nil {
			panic(err)
		}
		p, err := tree.Route(acct)
		if err != nil {
			panic(err)
		}
		count[p]++
	}
	fmt.Printf("routed %d random lite accounts:\n", N)
	for _, p := range []string{"BVN1", "BVN2", "BVN3"} {
		fmt.Printf("  %-6s %6d  %5.1f%%\n", p, count[p], 100*float64(count[p])/N)
	}
}
