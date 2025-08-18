// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package client_test

import (
	"context"
	"fmt"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/client"
)

// ExampleClient_GetAccount demonstrates querying an account from mainnet.
func ExampleClient_GetAccount() {
	// Create a client connected to mainnet
	c, err := client.NewMainnet()
	if err != nil {
		fmt.Printf("Failed to create client: %v\n", err)
		return
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Query the ACME token account
	account, err := c.GetAccount(ctx, "acc://ACME")
	if err != nil {
		fmt.Printf("Failed to get account: %v\n", err)
		return
	}

	// Print account type
	if account != nil && account.Account != nil {
		fmt.Printf("Account type: %s\n", account.Account.Type())
	}

	// Output:
	// Account type: tokenIssuer
}

// ExampleClient_GetNodeInfo demonstrates getting node information.
func ExampleClient_GetNodeInfo() {
	// Create a client connected to mainnet
	c, err := client.NewMainnet()
	if err != nil {
		fmt.Printf("Failed to create client: %v\n", err)
		return
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get node info
	info, err := c.GetNodeInfo(ctx)
	if err != nil {
		fmt.Printf("Failed to get node info: %v\n", err)
		return
	}

	// Check if we got network info
	if info != nil && info.Network != "" {
		fmt.Printf("Connected to network: %s\n", info.Network)
	}

	// Output:
	// Connected to network: MainNet
}
