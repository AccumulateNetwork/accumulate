package main

import (
	"flag"
	"fmt"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

var bvns = flag.Int("bvns", 0, "Number of BVNs")

func main() {
	flag.Parse()

	u, err := url.Parse(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v", err)
		os.Exit(1)
	}

	fmt.Printf("Account       : %v\n", u)
	fmt.Printf("Resource chain: %X\n", u.AccountID())
	fmt.Printf("Identity chain: %X\n", u.IdentityAccountID())
	fmt.Printf("Routing number: %X\n", u.Routing())
	if *bvns != 0 {
		fmt.Printf("Routes to     : BVN %d\n", u.Routing()%uint64(*bvns))
	}
}
