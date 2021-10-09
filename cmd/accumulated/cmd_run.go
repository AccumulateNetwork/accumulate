package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/AccumulateNetwork/accumulated"
	"github.com/AccumulateNetwork/accumulated/config"
	"github.com/AccumulateNetwork/accumulated/internal/abci"
	"github.com/AccumulateNetwork/accumulated/internal/api"
	"github.com/AccumulateNetwork/accumulated/internal/chain"
	"github.com/AccumulateNetwork/accumulated/internal/node"
	"github.com/AccumulateNetwork/accumulated/internal/relay"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/getsentry/sentry-go"
	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/privval"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
)

var cmdRun = &cobra.Command{
	Use:   "run",
	Short: "Run node",
	Run:   runNode,
}

var flagRun = struct {
	Node        int
	CiStopAfter time.Duration
}{}

//note: making FlagRunStruct an anonymous struct will cause go fmt to fail on Windows
//var flagRun flagRunStruct

func init() {
	cmdMain.AddCommand(cmdRun)

	cmdRun.Flags().IntVarP(&flagRun.Node, "node", "n", -1, "Which node are we? [0, n)")
	cmdRun.Flags().DurationVar(&flagRun.CiStopAfter, "ci-stop-after", 0, "FOR CI ONLY - stop the node after some time")
	cmdRun.Flag("ci-stop-after").Hidden = true
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

	if config.Accumulate.SentryDSN != "" {
		opts := sentry.ClientOptions{
			Dsn:           config.Accumulate.SentryDSN,
			Environment:   "Accumulate",
			HTTPTransport: sentryHack{},
			Debug:         true,
		}
		if accumulated.IsVersionKnown() {
			opts.Release = accumulated.Commit
		}
		err = sentry.Init(opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: configuring sentry: %v\n", err)
			os.Exit(1)
		}
		defer sentry.Flush(2 * time.Second)
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

	rpcClient, err := rpchttp.New(config.RPC.ListenAddress)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to reate RPC client: %v", err)
		os.Exit(1)
	}

	query := api.NewQuery(relay.New(rpcClient))
	mgr, err := chain.NewBlockValidator(query, db, pv.Key.PrivKey.Bytes())
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
	query = api.NewQuery(txRelay)

	api.StartAPI(&config.Accumulate.API, query)

	if flagRun.CiStopAfter == 0 {
		// Block forever
		select {}
	}

	time.Sleep(flagRun.CiStopAfter)

	// TODO I tried stopping the node, but node.Stop() won't return for a
	// follower node until it's caught up (I think).
}

type sentryHack struct{}

func (sentryHack) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host != "gitlab.com" {
		return http.DefaultTransport.RoundTrip(req)
	}

	defer func() {
		r := recover()
		if r != nil {
			log.Printf("Failed to send event to sentry: %v", r)
		}
	}()

	defer req.Body.Close()
	b, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}

	var v map[string]interface{}
	err = json.Unmarshal(b, &v)
	if err != nil {
		return nil, err
	}

	ex := v["exception"]

	// GitLab expects the event to have a different shape
	v["exception"] = map[string]interface{}{
		"values": ex,
	}

	b, err = json.Marshal(v)
	if err != nil {
		return nil, err
	}

	req.ContentLength = int64(len(b))
	req.Body = io.NopCloser(bytes.NewReader(b))
	resp, err := http.DefaultTransport.RoundTrip(req)
	return resp, err
}
