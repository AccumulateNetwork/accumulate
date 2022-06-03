package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/privval"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/client"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/networks"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	cmdMain.AddCommand(cmdSet)
	cmdSet.AddCommand(
		cmdSetOracle,
	)

	cmdSet.PersistentFlags().StringVarP(&flagSet.Server, "server", "s", "", "Override the API URL")
}

var flagSet = struct {
	Server string
}{}

var cmdSet = &cobra.Command{
	Use:   "set",
	Short: "Set network values",
}

var cmdSetOracle = &cobra.Command{
	Use:   "oracle [value]",
	Short: "Set the oracle",
	Args:  cobra.ExactArgs(1),
	Run:   setOracle,
}

func setOracle(_ *cobra.Command, args []string) {
	cfg, client := loadConfigAndClient()

	newValue, err := strconv.ParseFloat(args[0], 64)
	checkf(err, "oracle value")

	if cfg.Accumulate.Network.Type != config.Directory {
		fatalf("node is not a directory node")
	}

	oracle := new(protocol.DataAccount)
	req := new(api.GeneralQuery)
	req.Url = cfg.Accumulate.Network.NodeUrl(protocol.Oracle)
	_, err = client.QueryAccountAs(context.Background(), req, oracle)
	checkf(err, "get oracle")

	values := new(core.GlobalValues)
	err = values.ParseOracle(oracle.Entry)
	checkf(err, "parse oracle")

	values.Oracle.Price = uint64(newValue * protocol.AcmeOraclePrecision)
	transaction := new(protocol.Transaction)
	transaction.Header.Principal = cfg.Accumulate.Network.NodeUrl(protocol.Oracle)
	transaction.Body = &protocol.WriteData{
		Entry:        values.FormatOracle(),
		WriteToState: true,
	}

	submitTransactionWithNode(cfg, client, transaction, oracle)
}

func loadConfigAndClient() (*config.Config, *client.Client) {
	cfg, err := config.Load(flagMain.WorkDir)
	checkf(err, "--work-dir")

	server := flagSet.Server
	if server == "" {
		addr := cfg.Accumulate.Network.LocalAddress
		if !strings.Contains(addr, "://") {
			addr = "http://" + addr
		}
		u, err := config.OffsetPort(addr, networks.AccApiPortOffset)
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
	checkf(err, "get oracle signer")

	pv, err := privval.LoadFilePV(
		cfg.PrivValidator.KeyFile(),
		cfg.PrivValidator.StateFile(),
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
		Envelope: &protocol.Envelope{
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

	if result.Error != nil {
		fmt.Printf("Transaction failed\nHash: %x\nSignature: %x\n%+v\n", resp.TransactionHash, resp.SignatureHashes[0], result.Error)
		return
	}

	fmt.Printf("Transaction failed\nHash: %x\nSignature: %x\nCode: %d\n%s\n", resp.TransactionHash, resp.SignatureHashes[0], result.Code, result.Message)
}
