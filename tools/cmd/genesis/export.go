// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/cometbft/cometbft/crypto/ed25519"
	tmjson "github.com/cometbft/cometbft/libs/json"
	"github.com/cometbft/cometbft/types"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var cmdExport = &cobra.Command{
	Use:   "export [node] [network id] [exported genesis]",
	Short: "Export the network state to a new genesis document",
	Run:   export,
}

var flagExport = struct {
	FactomLDAs string
}{}

func init() {
	cmd.AddCommand(cmdExport)
	cmdExport.Flags().StringVar(&flagExport.FactomLDAs, "factom-ldas", "", "A snapshot containing the transaction history of Factom LDAs, for repairing the database")
}

func export(_ *cobra.Command, args []string) {
	factom := ldaCollector{}
	if flagExport.FactomLDAs != "" {
		f, err := os.Open(flagExport.FactomLDAs)
		check(err)
		check(snapshot.Visit(f, factom))
		check(f.Close())
	}

	nodeDir, netID, outPath := args[0], args[1], args[2]
	daemon, err := accumulated.Load(nodeDir, func(c *config.Config) (io.Writer, error) {
		return logging.NewConsoleWriter(c.LogFormat)
	})
	check(err)

	oldDoc, err := types.GenesisDocFromFile(daemon.Config.GenesisFile())
	check(err)

	db, err := database.Open(daemon.Config, daemon.Logger)
	check(err)

	batch := db.Begin(false)
	defer batch.Discard()

	var ledger *protocol.SystemLedger
	partUrl := config.NetworkUrl{URL: protocol.PartitionUrl(daemon.Config.Accumulate.PartitionId)}
	check(batch.Account(partUrl.Ledger()).Main().GetAs(&ledger))

	genesisTx, err := batch.Account(partUrl.Ledger()).MainChain().Inner().Entry(0)
	check(err)

	globals := new(core.GlobalValues)
	check(globals.Load(partUrl.URL, func(account *url.URL, target interface{}) error {
		return batch.Account(account).Main().GetAs(target)
	}))

	header := new(snapshot.Header)
	header.Height = ledger.Index
	header.Timestamp = ledger.Timestamp

	buf := new(ioutil2.Buffer)
	w, err := snapshot.Collect(batch, header, buf, snapshot.CollectOptions{
		Logger: daemon.Logger.With("module", "snapshot"),
		VisitAccount: func(acct *snapshot.Account) error {
			factom := factom[acct.Url.AccountID32()]
			if factom == nil {
				return nil
			}

			chains := map[string]*snapshot.Chain{}
			for _, c := range acct.Chains {
				chains[c.Name] = c
			}
			for _, factom := range factom.Chains {
				c := chains[factom.Name]
				if c == nil {
					daemon.Logger.Error("Skipping account, chain is missing", "account", acct.Url, "chain", factom.Name)
					continue
				}
				if c.Head.Count != factom.Head.Count+1 {
					daemon.Logger.Error("Skipping account, height doesn't match", "account", acct.Url, "chain", factom.Name, "factom", factom.Head.Count, "height", c.Head.Count)
					continue
				}
				fmt.Printf("Restoring %v %v chain history\n", acct.Url, factom.Name)
				head := c.Head
				c.Head = factom.Head
				c.MarkPoints = factom.MarkPoints
				c.AddEntry(genesisTx)
				if !bytes.Equal(c.Head.Anchor(), head.Anchor()) {
					return errors.InternalError.WithFormat("restoring Factom entries changed the anchor")
				}
			}
			return nil
		},
	})
	check(err)
	check(snapshot.CollectAnchors(w, batch, daemon.Config.Accumulate.PartitionUrl()))

	rootHash, err := batch.GetBptRootHash()
	check(err)

	doc := new(types.GenesisDoc)
	doc.InitialHeight = int64(ledger.Index) + 1
	doc.GenesisTime = ledger.Timestamp
	doc.ChainID = netID + "-" + daemon.Config.Accumulate.PartitionId
	doc.ConsensusParams = oldDoc.ConsensusParams
	doc.AppHash = rootHash[:]
	doc.AppState, err = json.Marshal(buf.Bytes())
	check(err)

	for _, val := range globals.Network.Validators {
		if !val.IsActiveOn(daemon.Config.Accumulate.PartitionId) {
			continue
		}

		key := ed25519.PubKey(val.PublicKey)
		genval := new(types.GenesisValidator)
		genval.Address = key.Address()
		genval.PubKey = key
		genval.Power = 1
		genval.Name = key.Address().String()
		doc.Validators = append(doc.Validators, *genval)
	}

	outFile, err := os.Create(outPath)
	check(err)
	defer outFile.Close()
	b, err := tmjson.MarshalIndent(doc, "", "  ")
	check(err)
	_, err = outFile.Write(b)
	check(err)
}

type ldaCollector map[[32]byte]*snapshot.Account

func (c ldaCollector) VisitAccount(acct *snapshot.Account, _ int) error {
	if acct == nil {
		return nil
	}

	if acct.Main.Type() == protocol.AccountTypeLiteDataAccount {
		c[acct.Url.AccountID32()] = acct
	}

	return nil
}
