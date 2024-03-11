// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/cmd/play-accumulate/pkg"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	testing "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

var Flag = struct {
	Network  string
	BvnCount int
}{}

var Command = &cobra.Command{
	Use:   "play-accumulate [files...]",
	Short: "Run Accumulate playbooks",
	Run:   run,
}

func init() {
	Command.Flags().StringVarP(&Flag.Network, "network", "n", "", "Run the test against a network")
	Command.Flags().IntVarP(&Flag.BvnCount, "bvns", "b", 3, "Number of BVNs to simulate")
}

func run(cmd *cobra.Command, filenames []string) {
	if cmd.Flag("network").Changed && cmd.Flag("bvns").Changed {
		fmt.Fprintf(os.Stderr, "Error: --network and --bvns are mutually exclusive\n")
		os.Exit(1)
	}

	testing.EnableDebugFeatures()

	var c *client.Client
	var err error
	if Flag.Network != "" {
		c, err = client.New(Flag.Network)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating client %q: %v\n", Flag.Network, err)
			os.Exit(1)
		}
	}

	var didFail bool
	for _, filename := range filenames {
		err := pkg.ExecuteFile(context.Background(), filename, Flag.BvnCount, c)
		if err != nil {
			didFail = true
		}
	}

	if didFail {
		os.Exit(1)
	}
}
