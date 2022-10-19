// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"

	"github.com/davecgh/go-spew/spew"
	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/rpc/client/http"
	"github.com/tendermint/tendermint/types"
)

var watchTxCmd = &cobra.Command{
	Use:   "watch-tx [addrs]",
	Short: "Subscribe to Tendermint transaction events",
	Args:  cobra.MinimumNArgs(1),
	RunE:  watchTx,
}

func init() {
	cmd.AddCommand(watchTxCmd)
}

func watchTx(_ *cobra.Command, args []string) error {
	for _, arg := range args {
		client, err := http.New(arg, arg + "/websocket")
		if err != nil {
			return fmt.Errorf("failed to create client: %v", err)
		}

		arg := arg // See docs/developer/rangevarref.md
		go func() {
			for {
				err = client.Start()
				if err != nil {
					fmt.Printf("Error: failed to start client: %v\n", err)
					return
				}

				txs, err := client.Subscribe(context.Background(), "watch-tx", "tm.event = 'Tx'")
				if err != nil {
					fmt.Printf("Error: failed to subscribe: %v\n", err)
					return
				}

				for e := range txs {
					if e.Data == nil {
						spew.Dump(e)
						break
					}

					data := e.Data.(types.EventDataTx)
					fmt.Printf("[%s] code=%d log=%q", arg, data.Result.Code, data.Result.Log)
					for key, values := range e.Events {
						for _, value := range values {
							// if e.Type == "tx" && a.Key == "hash" {
							// 	a.Value = a.Value[:8]
							// }
							fmt.Printf(" %s=%s", key, value)
						}
					}
					fmt.Println()
				}
			}
		}()
	}

	// Block forever
	select {}
}
