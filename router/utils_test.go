package router

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/AccumulateNetwork/accumulated/networks"

	"github.com/AccumulateNetwork/accumulated/config"
	"github.com/AccumulateNetwork/accumulated/internal/abci"
	"github.com/AccumulateNetwork/accumulated/internal/chain"
	"github.com/AccumulateNetwork/accumulated/internal/node"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/spf13/viper"
	tmnet "github.com/tendermint/tendermint/libs/net"
	"github.com/tendermint/tendermint/privval"
)

func randomRouterPorts() *config.Router {
	port, err := tmnet.GetFreePort()
	if err != nil {
		panic(err)
	}
	return &config.Router{
		JSONListenAddress: fmt.Sprintf("localhost:%d", port),
		RESTListenAddress: fmt.Sprintf("localhost:%d", port+1),
	}
}

func boostrapBVC(configfile string, workingdir string, baseport int) error {
	idx := networks.IndexOf("Localhost")
	opts := node.InitOptions{}
	opts.WorkDir = workingdir
	opts.Port = networks.Networks[idx].Port
	opts.ListenIP = networks.Networks[idx].Ip
	node.Init(opts)

	viper.SetConfigFile(configfile)
	viper.AddConfigPath(filepath.Join(workingdir, "Node0"))
	viper.ReadInConfig()
	//[mempool]
	//	broadcast = true
	//	cache_size = 100000
	//	max_batch_bytes = 10485760
	//	max_tx_bytes = 1048576
	//	max_txs_bytes = 1073741824
	//	recheck = true
	//	size = 50000
	//	wal_dir = ""
	//

	//viper.Set("mempool.keep-invalid-txs-in-cache, "false"
	//viper.Set("mempool.max_txs_bytes", "1073741824")
	viper.Set("mempool.max_batch_bytes", 1048576)
	viper.Set("mempool.cache_size", 1048576)
	viper.Set("mempool.size", 50000)
	err := viper.WriteConfig()
	if err != nil {
		panic(err)
	}

	return nil
}

func newBVC(t *testing.T, configfile string, workingdir string) (*config.Config, *privval.FilePV, *node.Node) {
	cfg, err := config.LoadFile(workingdir, configfile)
	if err != nil {
		panic(err)
	}

	dbPath := filepath.Join(cfg.RootDir, "valacc.db")
	bvcId := sha256.Sum256([]byte(cfg.Instrumentation.Namespace))
	sdb := new(state.StateDB)
	err = sdb.Open(dbPath, bvcId[:], false, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to open database %s: %v", dbPath, err)
		os.Exit(1)
	}

	// read private validator
	pv, err := privval.LoadFilePV(
		cfg.PrivValidator.KeyFile(),
		cfg.PrivValidator.StateFile(),
	)
	if err != nil {
		t.Fatal(err)
	}

	bvc := chain.NewBlockValidator()
	mgr, err := chain.NewManager(cfg, sdb, pv.Key.PrivKey.Bytes(), bvc)
	if err != nil {
		t.Fatal(err)
	}

	app, err := abci.NewAccumulator(sdb, pv.Key.PubKey.Address(), mgr)
	if err != nil {
		t.Fatal(err)
	}

	node, err := node.New(cfg, app)
	if err != nil {
		t.Fatal(err)
	}
	return cfg, pv, node
}

func startBVC(t *testing.T, cfgPath string, dir string) (*config.Config, *privval.FilePV, *node.Node) {

	//Select a base port to open.  Ports 43210, 43211, 43212, 43213,43214 need to be open
	baseport := 35550

	//generate the config files needed to run a test BVC
	err := boostrapBVC(cfgPath, dir, baseport)
	if err != nil {
		t.Fatal(err)
	}

	///Build a BVC we'll use for our test
	cfg, pv, node := newBVC(t, cfgPath, filepath.Join(dir, "Node0"))
	err = node.Start()
	if err != nil {
		t.Fatal(err)
	}

	return cfg, pv, node
}
