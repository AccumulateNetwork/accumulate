package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/cmd/play-accumulate/pkg"
	"gitlab.com/accumulatenetwork/accumulate/internal/client"
	testing "gitlab.com/accumulatenetwork/accumulate/internal/testing"
)

var Flag = struct {
	Network string
}{}

var Command = &cobra.Command{
	Use:   "play-accumulate [files...]",
	Short: "Run Accumulate playbooks",
	Run:   run,
}

func init() {
	Command.Flags().StringVarP(&Flag.Network, "network", "n", "", "Run the test against a network")
}

func run(_ *cobra.Command, filenames []string) {
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
		err := pkg.ExecuteFile(filename, c)
		if err != nil {
			didFail = true
		}
	}

	if didFail {
		os.Exit(1)
	}
}
