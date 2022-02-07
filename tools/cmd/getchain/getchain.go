package main

import (
	"fmt"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

func main() {
	u, err := url.Parse(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v", err)
		os.Exit(1)
	}

	fmt.Printf("Resource chain: %X\n", u.AccountID())
	fmt.Printf("Identity chain: %X\n", u.IdentityAccountID())
}
