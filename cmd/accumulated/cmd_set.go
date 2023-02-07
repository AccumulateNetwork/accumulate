// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	neturl "net/url"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	cmdMain.AddCommand(cmdSet)
	cmdSet.AddCommand(
		cmdSetOracle,
		cmdSetSchedule,
		cmdSetAnchorEmptyBlocks,
		cmdSetRouting,
	)

	cmdSet.PersistentFlags().StringVarP(&flagSet.Server, "server", "s", "", "Override the API URL")
	cmdSet.PersistentFlags().BoolVarP(&flagSet.Yes, "yes", "y", false, "Skip prompts (answer yes)")
}

var flagSet = struct {
	Server  string
	Suggest bool
	Yes     bool
}{}

var cmdSet = &cobra.Command{
	Use:   "set",
	Short: "Set network values",
}

var cmdSetOracle = &cobra.Command{
	Use:   "oracle [value]",
	Short: "Set the oracle",
	Args:  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		newValue, err := strconv.ParseFloat(args[0], 64)
		checkf(err, "oracle value")

		setNetworkValue(protocol.Oracle, func(v *core.GlobalValues) {
			v.Oracle.Price = uint64(newValue * protocol.AcmeOraclePrecision)
		})
	},
}

var cmdSetSchedule = &cobra.Command{
	Use:   "schedule [CRON expression]",
	Short: "Set the major block schedule",
	Args:  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		_, err := core.Cron.Parse(args[0])
		checkf(err, "CRON expression is invalid")

		setNetworkValue(protocol.Globals, func(v *core.GlobalValues) {
			v.Globals.MajorBlockSchedule = args[0]
		})
	},
}

var cmdSetAnchorEmptyBlocks = &cobra.Command{
	Use:   "anchor-empty-blocks [true|false]",
	Short: "Set whether empty blocks are anchored",
	Args:  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		value, err := strconv.ParseBool(args[0])
		checkf(err, "value is invalid")

		setNetworkValue(protocol.Globals, func(v *core.GlobalValues) {
			v.Globals.AnchorEmptyBlocks = value
		})
	},
}

var cmdSetRouting = &cobra.Command{
	Use:   "routing [table]",
	Short: "Set the routing table",
	Args:  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		table := new(protocol.RoutingTable)
		err := json.Unmarshal([]byte(args[0]), table)
		checkf(err, "value is invalid")

		if !flagSet.Yes {
			fmt.Printf("Updating the network's routing table:\n")
			for _, o := range table.Overrides {
				fmt.Printf("Override: %v <- %v\n", o.Partition, o.Account)
			}
			for _, r := range table.Routes {
				v := strconv.FormatUint(r.Value, 2)
				v = strings.Repeat("0", int(r.Length)-len(v)) + v
				fmt.Printf("Prefix:   %v <- %v\n", r.Partition, v)
			}
			fmt.Printf("Proceed [yN]? ")
			answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
			if err != nil {
				return
			}
			answer = strings.ToLower(strings.TrimSpace(answer))
			if answer != "y" && answer != "yes" {
				return
			}
		}

		setNetworkValue(protocol.Routing, func(v *core.GlobalValues) {
			v.Routing = table
		})
	},
}

func setNetworkValue(path string, update func(v *core.GlobalValues)) {
	cfg, client := loadConfigAndClient()
	if cfg.Accumulate.NetworkType != config.Directory {
		fatalf("node is not a directory node")
	}

	account := new(protocol.DataAccount)
	req := new(api.GeneralQuery)
	req.Url = cfg.Accumulate.Describe.NodeUrl(path)
	_, err := client.QueryAccountAs(context.Background(), req, account)
	checkf(err, "get %s", path)

	values := new(core.GlobalValues)
	var format func() protocol.DataEntry
	switch path {
	case protocol.Oracle:
		err = values.ParseOracle(account.Entry)
		format = values.FormatOracle
	case protocol.Globals:
		err = values.ParseGlobals(account.Entry)
		format = values.FormatGlobals
	case protocol.Network:
		err = values.ParseNetwork(account.Entry)
		format = values.FormatNetwork
	case protocol.Routing:
		err = values.ParseRouting(account.Entry)
		format = values.FormatRouting
	default:
		fatalf("unknown network variable account %s", path)
	}
	checkf(err, "parse %s", path)

	update(values)

	wd := new(protocol.WriteData)
	wd.WriteToState = true
	wd.Entry = format()
	transaction := new(protocol.Transaction)
	transaction.Header.Principal = cfg.Accumulate.Describe.NodeUrl(path)
	transaction.Body = wd

	submitTransactionWithNode(cfg, client, transaction, account)
}

func loadConfigAndClient() (*config.Config, *client.Client) {
	cfg, err := config.Load(flagMain.WorkDir)
	checkf(err, "--work-dir")

	server := flagSet.Server
	if server == "" {
		addr := cfg.Accumulate.LocalAddress
		if !strings.Contains(addr, "://") {
			addr = "http://" + addr
		}
		u, err := neturl.Parse(addr)
		checkf(err, "invalid address")

		port, err := strconv.ParseInt(u.Port(), 0, 16)
		checkf(err, "invalid port number on address")

		u, err = config.OffsetPort(addr, int(port), int(config.PortOffsetAccumulateApi))
		checkf(err, "applying offset to node's local address")
		server = u.String()
	}

	client, err := client.New(server)
	checkf(err, "--server")

	return cfg, client
}

func submitTransactionWithNode(cfg *config.Config, client *client.Client, transaction *protocol.Transaction, account protocol.FullAccount) {
	signer := new(protocol.KeyPage)
	req := new(api.GeneralQuery)
	req.Url = account.GetAuth().KeyBook().JoinPath("1")
	_, err := client.QueryAccountAs(context.Background(), req, signer)
	checkf(err, "get signer")

	pv, err := config.LoadFilePV(
		cfg.PrivValidatorKeyFile(),
		cfg.PrivValidatorStateFile(),
	)
	checkf(err, "load private validator")

	signature, err := new(signing.Builder).
		UseSimpleHash().
		SetType(protocol.SignatureTypeED25519).
		SetUrl(signer.Url).
		SetVersion(signer.Version).
		SetPrivateKey(pv.Key.PrivKey.Bytes()).
		SetTimestampToNow().
		Initiate(transaction)
	checkf(err, "sign transaction")

	resp, err := client.ExecuteDirect(context.Background(), &api.ExecuteRequest{
		Envelope: &messaging.Envelope{
			Transaction: []*protocol.Transaction{transaction},
			Signatures:  []protocol.Signature{signature},
		},
	})
	checkf(err, "submit transaction")

	if resp.Code == 0 {
		fmt.Printf("Submitted transaction\nHash: %x\nSignature: %x\n", resp.TransactionHash, resp.SignatureHashes[0])
		return
	}

	data, err := json.Marshal(resp.Result)
	checkf(err, "remarshal result")
	result := new(protocol.TransactionStatus)
	err = json.Unmarshal(data, result)
	checkf(err, "unmarshal result")
	fmt.Printf("Transaction failed\nHash: %x\nSignature: %x\n%+v\n", resp.TransactionHash, resp.SignatureHashes[0], result.Error)
}
