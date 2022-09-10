package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/rpc/client/http"
	"github.com/tendermint/tendermint/types"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
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
		client, err := http.New(arg)
		if err != nil {
			return fmt.Errorf("failed to create client: %v", err)
		}

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
					data, ok := e.Data.(types.EventDataTx)
					if !ok {
						continue
					}

					env := new(protocol.Envelope)
					if env.UnmarshalBinary(data.Tx) != nil {
						continue
					}

					b, err := json.Marshal(env)
					if err != nil {
						continue
					}

					fmt.Println(string(b))
				}
			}
		}()
	}

	// Block forever
	select {}
}
