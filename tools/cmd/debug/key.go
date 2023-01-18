// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
)

var keyCmd = &cobra.Command{
	Use:   "key <address>",
	Short: "Print out everything about a key",
	Args:  cobra.ExactArgs(1),
	Run:   keyInfo,
}

func init() {
	cmd.AddCommand(keyCmd)
}

func keyInfo(_ *cobra.Command, args []string) {
	addr, err := address.Parse(args[0])
	check(err)

	fmt.Printf("Type:\t\t\t%s\n", addr.GetType())
	if v, ok := addr.GetPublicKeyHash(); ok {
		fmt.Printf("Public Key Hash:\t%x\n", v)
	}
	if v, ok := addr.GetPublicKey(); ok {
		fmt.Printf("Public Key:\t\t%x\n", v)
	}
	if v, ok := addr.GetPrivateKey(); ok {
		fmt.Printf("Private Key:\t\t%x\n", v)
	}
}
