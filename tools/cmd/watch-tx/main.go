package main

import (
	"context"
	"fmt"

	"github.com/davecgh/go-spew/spew"
	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/rpc/client/http"
	"github.com/tendermint/tendermint/types"
)

func main() {
	cmd := &cobra.Command{
		Use:  "watch-tx [addrs]",
		Args: cobra.MinimumNArgs(1),
		RunE: run,
	}

	_ = cmd.Execute()
}

func run(cmd *cobra.Command, args []string) error {
	for _, arg := range args {
		client, err := http.New(arg)
		if err != nil {
			return fmt.Errorf("failed to create client: %v", err)
		}

		arg := arg
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
					for _, e := range e.Events {
						for _, a := range e.Attributes {
							// if e.Type == "tx" && a.Key == "hash" {
							// 	a.Value = a.Value[:8]
							// }
							fmt.Printf(" %s.%s=%s", e.Type, a.Key, a.Value)
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
