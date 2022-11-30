// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var cmdRunSim = &cobra.Command{
	Use:     "simulator",
	Aliases: []string{"sim"},
	Short:   "Run an Accumulate network simulation",
	Args:    cobra.NoArgs,
	Run:     runSim,
}

var flagRunSim = struct {
	BvnCount       int
	ApiListen      string
	BlockFrequency time.Duration
}{}

func init() {
	cmdRun.AddCommand(cmdRunSim)

	cmdRunSim.Flags().IntVarP(&flagRunSim.BvnCount, "bvns", "b", 3, "Number of BVNs to simulate")
	cmdRunSim.Flags().StringVarP(&flagRunSim.ApiListen, "listen", "l", "127.0.1.1:26660", "API server listen address")
	cmdRunSim.Flags().DurationVarP(&flagRunSim.BlockFrequency, "frequency", "f", time.Second/10, "Block frequency")
}

func runSim(*cobra.Command, []string) {
	s := simulator.New(simTb{}, flagRunSim.BvnCount)
	s.InitFromGenesis()

	t := time.NewTicker(flagRunSim.BlockFrequency)
	defer t.Stop()
	go func() {
		for range t.C {
			var count int
			ch := make(chan *protocol.TransactionStatus, 100)
			go func() {
				for range ch {
					count++
				}
			}()

			start := time.Now()
			s.ExecuteBlock(ch)
			if count == 0 {
				continue
			}

			duration := time.Since(start)
			fmt.Printf("Executed % 4d transactions in %v = %.2f TPS\n", count, duration, float64(count)/duration.Seconds())
		}
	}()

	fmt.Fprintf(os.Stderr, "Listening on http://%s\n", flagRunSim.ApiListen)
	check(http.ListenAndServe(flagRunSim.ApiListen, s.NewServer()))
}

type simTb struct{}

func (simTb) Name() string         { return "Simulator" }
func (simTb) Log(a ...interface{}) { fmt.Println(a...) }
func (simTb) Fail()                {} // The simulator never exits so why record anything?
func (simTb) FailNow()             { os.Exit(1) }
func (simTb) Helper()              {}
