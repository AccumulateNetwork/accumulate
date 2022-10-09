package main

import (
	"context"
	"fmt"
	"log"
	stdurl "net/url"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var healCmd = &cobra.Command{
	Use:  "heal [network]",
	Args: cobra.MinimumNArgs(1),
	Run:  heal,
}

func init() {
	cmd.AddCommand(healCmd)
	healCmd.Flags().BoolVar(&healFlag.Continuous, "continuous", false, "Run healing in a loop every second")
}

var healFlag = struct {
	Continuous bool
}{}

func heal(_ *cobra.Command, args []string) {
	netu, err := stdurl.Parse(args[0])
	checkf(err, "network url")
	netp, err := strconv.ParseUint(netu.Port(), 10, 64)
	checkf(err, "network url port")
	netc, err := client.New(fmt.Sprintf("http://%s:%d", netu.Hostname(), netp+uint64(config.PortOffsetAccumulateApi)+config.PortOffsetDirectory))
	checkf(err, "network client")
	g, nodes := walkNetwork(args)

	partNodes := map[string][]*NodeData{}
	for _, part := range g.Network.Partitions {
		var pn []*NodeData
		for _, node := range nodes {
			if node.Hostname != "" && node.Info != nil && node.Info.IsActiveOn(part.ID) {
				pn = append(pn, node)
			}
		}
		if uint64(len(pn)) < g.ValidatorThreshold(part.ID) {
			fatalf("Error: insufficient nodes for %s; threshold is %d but I only found the address of %d nodes", part.ID, g.ValidatorThreshold(part.ID), len(pn))
		}
		partNodes[part.ID] = pn
	}

	for {
		for _, part := range g.Network.Partitions {
			if part.Type != protocol.PartitionTypeBlockValidator {
				continue
			}

			partc := partNodes[part.ID][0].AccumulateAPI(config.PortOffsetBlockValidator)
			partUrl := protocol.PartitionUrl(part.ID)
			ledger := getLedger(netc, partUrl)

			for _, txid := range ledger.Anchor(protocol.DnUrl()).Pending {
				res, err := netc.QueryTx(context.Background(), &api.TxnQuery{TxIdUrl: txid})
				if err != nil {
					log.Printf("Failed to query %v: %v\n", txid, err)
					continue
				}

				if uint64(len(res.Status.AnchorSigners)) >= g.ValidatorThreshold(protocol.Directory) {
					continue // Already have enough signers
				}

				fmt.Printf("Healing anchor %v\n", txid)

				signed := map[[32]byte]bool{}
				for _, s := range res.Status.AnchorSigners {
					signed[*(*[32]byte)(s)] = true
				}

				for _, node := range partNodes[protocol.Directory] {
					if signed[*(*[32]byte)(node.Info.PublicKey)] {
						continue
					}

					csrc := node.AccumulateAPI(config.PortOffsetDirectory)
					querySynthAndExecute(csrc, partc, protocol.DnUrl(), partUrl, res.Status.SequenceNumber)
				}
			}
		}

		{
			dnc := partNodes[protocol.Directory][0].AccumulateAPI(config.PortOffsetDirectory)
			ledger := getLedger(netc, protocol.DnUrl())

			for _, part := range g.Network.Partitions {
				if part.Type != protocol.PartitionTypeBlockValidator {
					continue
				}

				partUrl := protocol.PartitionUrl(part.ID)
				for _, txid := range ledger.Anchor(partUrl).Pending {
					res, err := netc.QueryTx(context.Background(), &api.TxnQuery{TxIdUrl: txid})
					if err != nil {
						log.Printf("Failed to query %v: %v\n", txid, err)
						continue
					}

					if uint64(len(res.Status.AnchorSigners)) >= g.ValidatorThreshold(part.ID) {
						continue // Already have enough signers
					}

					fmt.Printf("Healing anchor %v\n", txid)

					signed := map[[32]byte]bool{}
					for _, s := range res.Status.AnchorSigners {
						signed[*(*[32]byte)(s)] = true
					}

					// Collect signatures and the transaction
					for _, node := range partNodes[part.ID] {
						if signed[*(*[32]byte)(node.Info.PublicKey)] {
							continue
						}

						csrc := node.AccumulateAPI(config.PortOffsetBlockValidator)
						querySynthAndExecute(csrc, dnc, partUrl, protocol.DnUrl(), res.Status.SequenceNumber)
					}
				}
			}
		}
		if !healFlag.Continuous {
			break
		}
		time.Sleep(time.Second)
	}
}

func getLedger(c *client.Client, part *url.URL) *protocol.AnchorLedger {
	ledger := new(protocol.AnchorLedger)
	res := new(api.ChainQueryResponse)
	res.Data = ledger
	req := new(api.GeneralQuery)
	req.Url = part.JoinPath(protocol.AnchorPool)
	err := c.RequestAPIv2(context.Background(), "query", req, res)
	checkf(err, "query %s anchor ledger", part)
	return ledger
}
