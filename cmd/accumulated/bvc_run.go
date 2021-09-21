package main

import (
	"fmt"
	"path/filepath"

	"github.com/AccumulateNetwork/accumulated/blockchain/accumulate"
	"github.com/AccumulateNetwork/accumulated/router"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cmdBvcRun = &cobra.Command{
	Use:   "run",
	Short: "Run BVC node",
	Run:   runBVC,
}

var flagBvcRun struct {
	Node int
}

func init() {
	cmdBvc.AddCommand(cmdBvcRun)

	cmdBvcRun.Flags().IntVarP(&flagBvcRun.Node, "node", "n", -1, "Which node are we? Required [0, n)")

	cmdBvcRun.MarkFlagRequired("node")
}

func runBVC(*cobra.Command, []string) {
	nodeDir := fmt.Sprintf("Node%d", flagBvcRun.Node)
	workDir := filepath.Join(flagMain.WorkDir, nodeDir)
	configFile := filepath.Join(workDir, "config", "config.toml")

	fmt.Printf("%s\n", configFile)

	//First create a router
	viper.SetConfigFile(configFile)
	viper.AddConfigPath(workDir)
	viper.ReadInConfig()
	urlrouter := router.NewRouter(viper.GetString("accumulate.RouterAddress"))

	//Next create a BVC
	accvm, err := accumulate.CreateAccumulateBlockValidatorChain(configFile, workDir)
	if err != nil {
		panic(fmt.Errorf("unable to create accumulate BVC"))
	}

	///we really need to open up ports to ALL shards in the system.  Maybe this should be a query to the DBVC blockchain.
	accvmapi, _ := accvm.GetAPIClient()
	_ = urlrouter.AddBVCClient(accvm.GetName(), accvmapi)

	//the query object connects to the BVC, will be replaced with network client router
	query := router.NewQuery(accvm)

	go router.StartAPI(34000, query)

	var done chan struct{}
	<-done
}
