package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/AccumulateNetwork/accumulated/blockchain/accumulate"
	"github.com/AccumulateNetwork/accumulated/config"
	"github.com/AccumulateNetwork/accumulated/networks"
	"github.com/AccumulateNetwork/accumulated/router"
	"github.com/spf13/cobra"
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

	cmdRun.Flags().IntVarP(&flagRun.Node, "node", "n", -1, "Which node are we? [0, n)")
}

func runNode(cmd *cobra.Command, args []string) {
	workDir := flagMain.WorkDir
	if cmd.Flag("node").Changed {
		nodeDir := fmt.Sprintf("Node%d", flagRun.Node)
		workDir = filepath.Join(workDir, nodeDir)
	} else if !cmd.Flag("work-dir").Changed {
		fmt.Fprint(os.Stderr, "Error: at least one of --work-dir or --node is required\n")
		_ = cmd.Usage()
		os.Exit(1)
	}

	config, err := config.Load(workDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: reading config file: %v\n", err)
		os.Exit(1)
	}

	_, err = accumulate.CreateAccumulateBVC(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: creating BVC: %v\n", err)
		os.Exit(1)
	}

	///we really need to open up ports to ALL shards in the system.  Maybe this should be a query to the DBVC blockchain.
	networkList := []int{3}
	txBouncer := networks.MakeBouncer(networkList)

	//the query object connects to the BVC, will be replaced with network client router
	query := router.NewQuery(txBouncer)

	router.StartAPI(&config.Accumulate.Router, query, txBouncer)

	//Block forever
	select {}
}
