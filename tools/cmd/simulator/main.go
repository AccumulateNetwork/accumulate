package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/accumulated"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

var cmd = &cobra.Command{
	Use:   "simulator",
	Short: "Accumulate network simulator",
	Run:   run,
}

var flag = struct {
	Database string
	Network  string
	Snapshot string
	Log      string
	Step     string
	BvnCount int
	ValCount int
	BasePort int
}{}

func init() {
	cmd.Flags().StringVarP(&flag.Database, "database", "d", "memory", "The directory to store badger databases in, or 'memory' to use an in-memory database")
	cmd.Flags().StringVarP(&flag.Network, "network", "n", "simple", "A file used to define the network, or 'simple' to use a simple predefined network")
	cmd.Flags().StringVar(&flag.Snapshot, "snapshot", "genesis", "A directory containing snapshots used to initialize the network, or 'genesis' to initialize from genesis")
	cmd.Flags().StringVarP(&flag.Step, "step", "s", "on-wait", "Frequency at which to step the simulator, or 'on-wait' to step when an API query waits for a transaction")
	cmd.Flags().StringVar(&flag.Log, "log", DefaultLogLevels, "Log levels")
	cmd.Flags().IntVarP(&flag.BvnCount, "bvns", "b", 3, "Number of BVNs to create; applicable only when --network=simple")
	cmd.Flags().IntVarP(&flag.ValCount, "validators", "v", 3, "Number of validators to create per BVN; applicable only when --network=simple")
	cmd.Flags().IntVarP(&flag.BasePort, "port", "p", 26656, "Base port to listen on")
}

var DefaultLogLevels = config.LogLevel{}.
	Parse(config.DefaultLogLevels).
	SetModule("sim", "info").
	String()

func main() { _ = cmd.Execute() }

func run(*cobra.Command, []string) {
	jsonrpc2.DebugMethodFunc = true

	var db simulator.OpenDatabaseFunc
	if flag.Database == "memory" {
		db = simulator.MemoryDatabase
	} else {
		db = simulator.BadgerDatabaseFromDirectory(flag.Database, func(err error) { checkf(err, "--database") })
	}

	var net *accumulated.NetworkInit
	if flag.Network == "simple" {
		net = simulator.SimpleNetwork("Simulator", flag.BvnCount, flag.ValCount)
		for i, bvn := range net.Bvns {
			for j, node := range bvn.Nodes {
				node.AdvertizeAddress = fmt.Sprintf("127.0.1.%d", 1+i*flag.ValCount+j)
				node.BasePort = uint64(flag.BasePort)
			}
		}
	} else {
		f, err := os.Open(flag.Network)
		checkf(err, "--network")
		err = json.NewDecoder(f).Decode(&net)
		checkf(err, "--network")
		err = f.Close()
		checkf(err, "--network")
	}

	var snapshot simulator.SnapshotFunc
	if flag.Snapshot == "genesis" {
		snapshot = simulator.Genesis(time.Now())
	} else {
		snapshot = simulator.SnapshotFromDirectory(flag.Snapshot)
	}

	logw, err := logging.NewConsoleWriter("plain")
	check(err)
	level, writer, err := logging.ParseLogLevel(flag.Log, logw)
	checkf(err, "--log")
	logger, err := logging.NewTendermintLogger(zerolog.New(writer), level, false)
	check(err)

	sim, err := simulator.New(logger, db, net, snapshot)
	check(err)

	if flag.Step == "on-wait" {
		check(sim.ListenAndServe(onWaitHook))
		return
	}

	step, err := time.ParseDuration(flag.Step)
	checkf(err, "--step")
	go func() {
		t := time.NewTicker(step)
		defer t.Stop()
		for range t.C {
			check(sim.Step())
		}
	}()

	check(sim.ListenAndServe(nil))
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

func onWaitHook(sim *simulator.Simulator, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2" {
			h.ServeHTTP(w, r)
			return
		}

		// Copy the body
		body, err := ioutil.ReadAll(r.Body)
		check(err)
		r2 := *r
		r = &r2
		r.Body = io.NopCloser(bytes.NewBuffer(body))

		var req jsonrpc2.Request
		if err := json.Unmarshal(body, &req); err != nil {
			h.ServeHTTP(w, r)
			return
		}

		if req.Method != "query-tx" {
			h.ServeHTTP(w, r)
			return
		}

		var params api.TxnQuery
		if err := json.Unmarshal(req.Params.(json.RawMessage), &params); err != nil {
			h.ServeHTTP(w, r)
			return
		}

		if params.Wait == 0 {
			h.ServeHTTP(w, r)
			return
		}

		if params.TxIdUrl == nil {
			waitForHash(sim, params.Txid, params.IgnorePending)
		} else {
			waitForTxID(sim, params.TxIdUrl, params.IgnorePending)
		}

		params.Wait = 0
		req.Params = &params
		body, err = json.Marshal(req)
		check(err)
		r.Body = io.NopCloser(bytes.NewBuffer(body))
		h.ServeHTTP(w, r)
	})
}

func waitForHash(sim *simulator.Simulator, hash []byte, ignorePending bool) {
	var ok bool
	for i := 0; !ok && i < 50; i++ {
		check(sim.ViewAll(func(batch *database.Batch) error {
			st, err := batch.Transaction(hash).Status().Get()
			check(err)
			ok = !ignorePending && st.Pending() || st.Delivered()
			return nil
		}))
		if !ok {
			check(sim.Step())
		}
	}
}

func waitForTxID(sim *simulator.Simulator, txid *url.TxID, ignorePending bool) {
	h := txid.Hash()
	var ok bool
	for i := 0; !ok && i < 50; i++ {
		check(sim.DatabaseFor(txid.Account()).View(func(batch *database.Batch) error {
			st, err := batch.Transaction(h[:]).Status().Get()
			check(err)
			ok = !ignorePending && st.Pending() || st.Delivered()
			return nil
		}))
		if !ok {
			check(sim.Step())
		}
	}
}
