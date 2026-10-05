// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// sizes measures, against a live network, the encoded size of chain entry
// records as a range answers them, and how long each answer takes.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func main() {
	c := jsonrpc.NewClient(os.Args[1])
	for _, a := range os.Args[2:] {
		var acct, chain string
		var start uint64
		var expand bool
		fmt.Sscanf(a, "%s %s %d %t", &acct, &chain, &start, &expand)
		u := url.MustParse(acct)
		count := uint64(100)
		t := time.Now()
		r, err := api.Querier2{Querier: c}.QueryChainEntries(context.Background(), u, &api.ChainQuery{Name: chain, Range: &api.RangeOptions{Start: start, Count: &count, Expand: &expand}})
		d := time.Since(t)
		if err != nil {
			fmt.Printf("%s#%s: %v\n", acct, chain, err)
			continue
		}
		var size int
		for _, e := range r.Records {
			b, err := e.MarshalBinary()
			if err != nil {
				panic(err)
			}
			size += len(b)
		}
		n := len(r.Records)
		if n == 0 {
			fmt.Printf("%s#%s: no records\n", acct, chain)
			continue
		}
		fmt.Printf("%-80s expand=%-5v %3d records  %6d B/record  %4.0f per 50KB  %v\n", acct+"#"+chain, expand, n, size/n, 50000/float64(size/n), d.Round(time.Millisecond))
	}
}
