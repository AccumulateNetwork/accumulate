// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/spf13/cobra"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/genesis"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/remote"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/test/testing"
)

var cmdGenesis = &cobra.Command{
	Use: "genesis",
}

var cmdGenesisIngest = &cobra.Command{
	Use:   "ingest [output] [inputs...]",
	Short: "Ingests multiple snapshots (one per partition) and merges them into the output database with all system data stripped",
	Args:  cobra.MinimumNArgs(2),
	Run:   ingestForGenesis,
}

func init() {
	cmd.AddCommand(cmdGenesis)
	cmdGenesis.AddCommand(cmdGenesisIngest)
}

const calculateBPT = false

func ingestForGenesis(cmd *cobra.Command, args []string) {
	// Timer for progress
	tick := time.NewTicker(time.Second / 2)
	defer tick.Stop()

	// Open the output database
	var store keyvalue.Beginner
	store, remoteAddr := openDbUrl(args[0], true)
	if remoteAddr != nil {
		store = remote.Connect(func() (io.ReadWriteCloser, error) {
			return net.Dial(remoteAddr.Network(), remoteAddr.String())
		})
	}

	db := coredb.New(store, nil)
	defer db.Close() // also closes store

	// Don't calculate BPT hashes since genesis doesn't want them
	db.SetObserver(testing.NullObserver{})

	// For more details on what gets changed, see [genesis.Extract]

	// For each input
	for _, path := range args[1:] {
		fmt.Println("Processing", path)
		file, err := os.Open(path)
		check(err)
		defer file.Close()
		_, err = genesis.Extract(db, file, func(u *url.URL) bool {
			select {
			case <-tick.C:
				// Print a progress message
				h := database.NewKey("Account", u).Hash()
				fmt.Printf("\033[A\r\033[KProcessing [%x] %v\n", h[:4], u)
			default:
				return true
			}

			// Retain everything
			return true
		})
		check(err)
	}
}
