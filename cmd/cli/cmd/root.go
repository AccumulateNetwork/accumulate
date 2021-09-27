package cmd

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/AccumulateNetwork/accumulated/client"
)

var (
	Client = client.NewAPIClient()
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = func() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "accumulate",
		Short: "CLI for Accumulate Network",
	}

	flags := cmd.PersistentFlags()
	flags.StringVar(&Client.Server, "server", "http://localhost:34000/v1", "Accumulated server")
	flags.DurationVar(&Client.Timeout, "timeout", 5*time.Second, "Timeout for all API requests (i.e. 10s, 1m)")
	flags.BoolVar(&Client.DebugRequest, "debug", false, "Print accumulated API calls")

	return cmd

}()

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	cobra.OnInitialize()
}
