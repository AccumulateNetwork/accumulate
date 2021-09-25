package main

import (
	"fmt"
	"path/filepath"

	"github.com/AccumulateNetwork/accumulated/blockchain/accumulate"
	"github.com/AccumulateNetwork/accumulated/networks"
	"github.com/AccumulateNetwork/accumulated/router"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cmdRun = &cobra.Command{
	Use:   "run",
	Short: "Run node",
	Run:   runNode,
}

var flagRun struct {
	Node int
}

func init() {
	cmdMain.AddCommand(cmdRun)

	cmdRun.Flags().IntVarP(&flagRun.Node, "node", "n", -1, "Which node are we? Required [0, n)")

	cmdRun.MarkFlagRequired("node")
}

func runNode(*cobra.Command, []string) {
	nodeDir := fmt.Sprintf("Node%d", flagRun.Node)

	workDir := filepath.Join(flagMain.WorkDir, nodeDir)
	configFile := filepath.Join(workDir, "config", "config.toml")

	fmt.Printf("%s\n", configFile)

	//First create a router
	viper.SetConfigFile(configFile)
	viper.AddConfigPath(workDir)
	err := viper.ReadInConfig()
	if err != nil {
		panic("failed to read config file")
	}
	//Next create a BVC
	_, err = accumulate.CreateAccumulateBVC(configFile, workDir)
	if err != nil {
		panic("failed create bvc")
	}

	///we really need to open up ports to ALL shards in the system.  Maybe this should be a query to the DBVC blockchain.

	networkList := []int{3}
	txBouncer := networks.MakeBouncer(networkList)

	//the query object connects to the BVC, will be replaced with network client router
	query := router.NewQuery(txBouncer)

	go router.StartAPI(34000, query, txBouncer)

	//Block forever
	select {}
}
