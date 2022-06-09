package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/url"
	"os"

	"github.com/spf13/cobra"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/client"
	"gitlab.com/accumulatenetwork/accumulate/networks"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var validatorsCmd = &cobra.Command{
	Use:   "validators <tendermint RPC address>",
	Short: "List the validators that are connected to a network",
	Run:   validators,
}

func init() {
	cmd.AddCommand(validatorsCmd)
}

func validators(_ *cobra.Command, args []string) {
	u, err := url.Parse(args[0])
	check(err)
	nodes := []string{u.String()}

	// Find peers
	{
		u, err := config.OffsetPort(u.String(), networks.TmRpcPortOffset)
		check(err)

		client, err := rpchttp.New(u.String())
		check(err)

		info, err := client.NetInfo(context.Background())
		check(err)

		for _, peer := range info.Peers {
			u, err := url.Parse(peer.URL)
			check(err)
			u.Scheme = "http"
			u.User = nil
			nodes = append(nodes, u.String())
		}
	}

	// Map peers to keys
	peers := map[[32]byte]string{}
	for _, addr := range nodes {
		u, err := config.OffsetPort(addr, networks.TmRpcPortOffset)
		check(err)

		client, err := rpchttp.New(u.String())
		check(err)

		status, err := client.Status(context.Background())
		check(err)

		if status.ValidatorInfo.PubKey == nil {
			continue
		}

		hash := sha256.Sum256(status.ValidatorInfo.PubKey.Bytes())
		peers[hash] = addr
	}

	// Describe the network
	u, err = config.OffsetPort(nodes[0], networks.AccApiPortOffset)
	check(err)

	client, err := client.New(u.String())
	check(err)

	describe, err := client.Describe(context.Background())
	check(err)

	type Subnet struct {
		ID          string
		GenesisRoot string
		Validators  []string
	}
	type Network struct {
		Name    string
		Subnets []*Subnet
	}

	network := new(Network)
	network.Name = describe.Network.NetworkName

	for _, subnet := range describe.Values.Network.Subnets {
		vals := new(Subnet)
		vals.ID = subnet.SubnetID
		network.Subnets = append(network.Subnets, vals)

		for _, hash := range subnet.ValidatorKeyHashes {
			peer, ok := peers[hash]
			if !ok {
				continue
			}

			if subnet.SubnetID != protocol.Directory {
				u, err := config.OffsetPort(peer, -networks.DnnPortOffset)
				check(err)
				peer = u.String()
			}

			vals.Validators = append(vals.Validators, peer)
		}

		if len(vals.Validators) == 0 {
			continue
		}

		u, err := config.OffsetPort(vals.Validators[0], networks.TmRpcPortOffset)
		check(err)

		client, err := rpchttp.New(u.String())
		check(err)

		genesis, err := client.Genesis(context.Background())
		check(err)
		vals.GenesisRoot = genesis.Genesis.AppHash.String()
	}

	check(json.NewEncoder(os.Stdout).Encode(network))
}
