package main

import (
	"fmt"
	"os"

	"github.com/AccumulateNetwork/accumulate/internal/url"
)

func main() {
	u, err := url.Parse(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v", err)
		os.Exit(1)
	}

	fmt.Printf("Resource chain: %X\n", u.ResourceChain())
	fmt.Printf("Identity chain: %X\n", u.IdentityChain())
}
