package main

import (
	"fmt"
	"os"
	"os/user"
	"path"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/tools/cmd/staking/app"
	"gitlab.com/accumulatenetwork/accumulate/tools/cmd/staking/network"
	"gitlab.com/accumulatenetwork/accumulate/tools/cmd/staking/sim"
)

func main() {
	_ = cmd.Execute()
}

var cmd = &cobra.Command{
	Use:   "staking",
	Short: "Run the staking app",
	Args:  cobra.NoArgs,
	Run:   run,
}

var flag = struct {
	Debug      bool
	Simulator  bool
	Network    string
	Parameters *url.URL
}{
	Parameters: protocol.AccountUrl("staking.acme", "parameters"),
}

var theClient *client.Client

func init() {
	cmd.PersistentFlags().BoolVarP(&flag.Debug, "debug", "d", false, "debug API requests")
	cmd.Flags().BoolVar(&flag.Simulator, "sim", false, "Use the simulator")
	cmd.PersistentFlags().StringVarP(&flag.Network, "net", "n", "https://testnet.accumulatenetwork.io/v2", "The network to connect to")
	cmd.PersistentFlags().Var(urlFlag{&flag.Parameters}, "params", "Staking parameters account URL")
	cmd.MarkFlagsMutuallyExclusive("sim", "net")

	cmd.PersistentPreRunE = func(*cobra.Command, []string) error {
		if !flag.Simulator {
			var err error
			theClient, err = client.New(flag.Network)
			if err != nil {
				return errors.Format(errors.StatusUnknownError, "--net: %w", err)
			}

			theClient.DebugRequest = flag.Debug
		}

		return nil
	}
}

func run(*cobra.Command, []string) {
	u, _ := user.Current()
	app.ReportDirectory = path.Join(u.HomeDir + "/StakingReports")
	err := os.MkdirAll(app.ReportDirectory, os.ModePerm)
	check(err)

	s := new(app.StakingApp)
	if flag.Simulator {
		sim := new(sim.Simulator)
		s.Run(sim)
		return
	}

	net := network.New(theClient, flag.Parameters)
	s.Run(net)
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

func check(err error) {
	if err != nil {
		fatalf("%v", err)
	}
}

func checkf(err error, format string, otherArgs ...interface{}) {
	if err != nil {
		fatalf(format+": %v", append(otherArgs, err)...)
	}
}
