// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"sync"

	"github.com/cometbft/cometbft/rpc/client/http"
	"github.com/spf13/cobra"
)

func init() {
	cmd.AddCommand(cmdComet)
	cmdComet.AddCommand(cmdCometDownloadGenesis)
}

var cmdComet = &cobra.Command{
	Use:   "comet",
	Short: "Utilities for CometBFT",
}

var cmdCometDownloadGenesis = &cobra.Command{
	Use:   "download-genesis [node] [output]",
	Short: "Download genesis file from a CometBFT node",
	Args:  cobra.ExactArgs(2),
	Run:   downloadCometGenesis,
}

func downloadCometGenesis(_ *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	f, err := os.Create(args[1])
	check(err)
	defer f.Close()

	// Get the first chunk
	fmt.Printf("Get first chunk\n")
	comet, err := http.New(args[0], args[0]+"/ws")
	check(err)
	r, err := comet.GenesisChunked(ctx, 0)
	checkf(err, "get first genesis chunk")
	b, err := base64.StdEncoding.DecodeString(r.Data)
	checkf(err, "decode first genesis chunk")
	_, err = f.Write(b)
	check(err)

	// If there's only one chunk, stop
	if r.TotalChunks == 1 {
		return
	}

	// Get peers
	peers, err := comet.NetInfo(ctx)
	check(err)

	u, err := url.Parse(args[0])
	check(err)

	addrs := []string{args[0]}
	for _, peer := range peers.Peers {
		addrs = append(addrs, fmt.Sprintf("http://%v:%v", peer.RemoteIP, u.Port()))
	}

	wg := new(sync.WaitGroup)                 // Wait group for results
	wg.Add(r.TotalChunks - 1)                 //
	work := make(chan uint, r.TotalChunks-1)  // Work queue
	chunks := make([][]byte, r.TotalChunks-1) // Results

	// Fill the work queue
	for i := 1; i < r.TotalChunks; i++ {
		work <- uint(i)
	}

	// Start a worker for each peer
	for _, addr := range addrs {
		addr := addr
		comet, err := http.New(addr, addr+"/ws")
		check(err)

		// Do work while there's work to do
		go func() {
			for i := range work {
				fmt.Printf("Get genesis chunk %d/%d from %s\n", i+1, r.TotalChunks, addr)

				rgen, err := comet.GenesisChunked(ctx, i)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: get genesis chunk %d from %s: %v\n", i+1, addr, err)
					work <- i // Return work
					continue
				}

				b, err := base64.StdEncoding.DecodeString(rgen.Data)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: decode genesis chunk %d from %s: %v\n", i+1, addr, err)
					work <- i // Return work
					continue
				}

				chunks[i-1] = b // Record the chunk
				wg.Done()       // Mark it done
			}
		}()
	}

	wg.Wait()   // Wait for all work to be done
	close(work) // Release the workers

	// Record the chunks
	for i, b := range chunks {
		if b == nil {
			fatalf("missing chunk %d", i)
		}
		_, err = f.Write(b)
		check(err)
	}
}
