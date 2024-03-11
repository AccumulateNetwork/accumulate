// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tmed25519 "github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/cometbft/cometbft/types"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/abci"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	cmdMain.AddCommand(cmdReset)
	cmdReset.AddCommand(cmdResetConsensus)

	cmdResetConsensus.Flags().BoolVar(&flagResetConsensus.EmptyAppState, "empty-app-state", false, "Leave the app state empty, do not capture a snapshot")
}

var cmdReset = &cobra.Command{
	Use: "reset",
}

var cmdResetConsensus = &cobra.Command{
	Use:   "consensus [node]",
	Short: "Resets the consensus state (UNSAFE)",
	Long:  "Wipes the consensus state and constructs a new genesis document representing the current state of the network",
	Args:  cobra.ExactArgs(1),
	Run:   resetConsensus,
}

var flagResetConsensus = struct {
	EmptyAppState bool
}{}

func resetConsensus(_ *cobra.Command, args []string) {
	// Load
	daemon, err := accumulated.Load(args[0], nil)
	checkf(err, "load node")

	genDoc, err := types.GenesisDocFromFile(daemon.Config.GenesisFile())
	checkf(err, "load current genesis document")

	db, err := coredb.Open(daemon.Config, daemon.Logger)
	checkf(err, "open database")

	batch := db.Begin(false)
	defer batch.Discard()
	var ledger *protocol.SystemLedger
	partUrl := protocol.PartitionUrl(daemon.Config.Accumulate.PartitionId)
	err = batch.Account(partUrl.JoinPath(protocol.Ledger)).Main().GetAs(&ledger)
	checkf(err, "load ledger")

	globals := new(network.GlobalValues)
	err = globals.Load(partUrl, func(account *url.URL, target interface{}) error {
		return batch.Account(account).Main().GetAs(target)
	})
	checkf(err, "load globals")

	// Rebuild the validator list
	genDoc.Validators = nil
	for i, validator := range globals.Network.Validators {
		if !validator.IsActiveOn(daemon.Config.Accumulate.PartitionId) {
			continue
		}

		key := tmed25519.PubKey(validator.PublicKey)
		name := fmt.Sprintf("Validator.%d", i+1)
		if validator.Operator != nil {
			name = validator.Operator.Authority
		}

		genDoc.Validators = append(genDoc.Validators, types.GenesisValidator{
			Name:    name,
			Address: key.Address(),
			PubKey:  key,
			Power:   1,
		})
	}

	// Rebuild the app state
	genDoc.InitialHeight = int64(ledger.Index) + 1
	genDoc.GenesisTime = ledger.Timestamp
	genDoc.ConsensusParams.Version.App = abci.Version

	hash, err := batch.GetBptRootHash()
	checkf(err, "get root hash")
	genDoc.AppHash = hash[:]

	// Collect a snapshot
	genDoc.AppState = nil
	if !flagResetConsensus.EmptyAppState {
		var metrics coredb.CollectMetrics
		buf := new(ioutil.Buffer)

		tick := time.NewTicker(time.Second / 2)
		defer tick.Stop()

		fmt.Println("Collecting...")
		err = db.Collect(buf, partUrl, &coredb.CollectOptions{
			// BuildIndex: true,
			Metrics: &metrics,
			Predicate: func(r database.Record) (bool, error) {
				// The sole purpose of this function is to print progress
				select {
				case <-tick.C:
				default:
					return true, nil
				}

				switch r.Key().Get(0) {
				case "Account":
					k := r.Key().SliceJ(2)
					h := k.Hash()
					fmt.Printf("\033[A\r\033[KCollecting [%x] (%d) %v\n", h[:4], metrics.Messages.Count, k.Get(1))

				case "Message", "Transaction":
					fmt.Printf("\033[A\r\033[KCollecting (%d/%d) %x\n", metrics.Messages.Collecting, metrics.Messages.Count, r.Key().Get(1).([32]byte))
				}

				// Retain everything
				return true, nil
			},
		})
		checkf(err, "collect snapshot")

		genDoc.AppState, err = json.Marshal(buf.Bytes())
		checkf(err, "marshal snapshot")
	}

	// Save genesis
	err = genDoc.SaveAs(daemon.Config.GenesisFile())
	checkf(err, "write genesis")

	// Erase the consensus history
	check(os.RemoveAll(filepath.Join(daemon.Config.DBDir(), "blockstore.db")))
	check(os.RemoveAll(filepath.Join(daemon.Config.DBDir(), "cs.wal")))
	check(os.RemoveAll(filepath.Join(daemon.Config.DBDir(), "evidence.db")))
	check(os.RemoveAll(filepath.Join(daemon.Config.DBDir(), "state.db")))
	check(os.RemoveAll(filepath.Join(daemon.Config.DBDir(), "tx_index.db")))
	check(os.WriteFile(
		filepath.Join(daemon.Config.DBDir(), "priv_validator_state.json"),
		[]byte(`{ "height": "0", "round": 0, "step": 0 }`),
		0600))
}
