package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"

	"github.com/AccumulateNetwork/accumulated/config"
	"github.com/AccumulateNetwork/accumulated/internal/abci"
	"github.com/AccumulateNetwork/accumulated/internal/api"
	"github.com/AccumulateNetwork/accumulated/internal/chain"
	"github.com/AccumulateNetwork/accumulated/internal/node"
	"github.com/AccumulateNetwork/accumulated/internal/relay"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/privval"
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

	dbPath := filepath.Join(config.RootDir, "valacc.db")
	bvcId := sha256.Sum256([]byte(config.Instrumentation.Namespace))
	db := new(state.StateDB)
	err = db.Open(dbPath, bvcId[:], false, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to open database %s: %v", dbPath, err)
		os.Exit(1)
	}

	// read private validator
	pv, err := privval.LoadFilePV(
		config.PrivValidator.KeyFile(),
		config.PrivValidator.StateFile(),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load private validator: %v", err)
		os.Exit(1)
	}

	bvc := chain.NewBlockValidator()
	mgr, err := chain.NewManager(config, db, pv.Key.PrivKey.Bytes(), bvc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to initialize chain manager: %v", err)
		os.Exit(1)
	}

	app, err := abci.NewAccumulator(db, pv.Key.PubKey.Address(), mgr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to initialize ACBI app: %v", err)
		os.Exit(1)
	}

	// Create node
	node, err := node.New(config, app)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to initialize node: %v\n", err)
		os.Exit(1)
	}

	// Start node
	err = node.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to start node: %v\n", err)
		os.Exit(1)
	}

	txRelay, err := relay.NewWith(config.Accumulate.Networks...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// The query object connects to the BVC, will be replaced with network client router
	query := api.NewQuery(txRelay)

	api.StartAPI(&config.Accumulate.API, query, txRelay)

	// Block forever
	select {}
}
