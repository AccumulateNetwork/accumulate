package main

import (
	"encoding/json"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/crypto/ed25519"
	tmjson "github.com/tendermint/tendermint/libs/json"
	"github.com/tendermint/tendermint/types"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/accumulated"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var cmdExport = &cobra.Command{
	Use:   "export [node] [exported genesis]",
	Short: "Export the network state to a new genesis document",
	Run:   export,
	Args:  cobra.ExactArgs(2),
}

var flagExport = struct {
	FactomLDAs  string
	NetworkName string
	Validators  string
}{}

func init() {
	testing.EnableDebugFeatures()
	cmd.AddCommand(cmdExport)
	cmdExport.Flags().StringVar(&flagExport.FactomLDAs, "factom-ldas", "", "A snapshot containing the transaction history of Factom LDAs, for repairing the database")
	cmdExport.Flags().StringVar(&flagExport.NetworkName, "network", "", "Change the name of the network")
	cmdExport.Flags().StringVar(&flagExport.Validators, "validators", "", "Overwrite the network definition's validator set (JSON)")
}

func export(_ *cobra.Command, args []string) {
	// Load the node
	nodeDir, outPath := args[0], args[1]
	daemon, err := accumulated.Load(nodeDir, func(c *config.Config) (io.Writer, error) {
		return logging.NewConsoleWriter(c.LogFormat)
	})
	check(err)

	// Load the old genesis document
	oldDoc, err := types.GenesisDocFromFile(daemon.Config.GenesisFile())
	check(err)

	// Open the database
	db, err := database.Open(daemon.Config, daemon.Logger)
	check(err)
	batch := db.Begin(false)
	defer batch.Discard()

	// Load the system ledger
	var ledger *protocol.SystemLedger
	partUrl := config.NetworkUrl{URL: protocol.PartitionUrl(daemon.Config.Accumulate.PartitionId)}
	check(batch.Account(partUrl.Ledger()).Main().GetAs(&ledger))

	// Load the global variables
	globals := new(core.GlobalValues)
	check(globals.Load(partUrl, func(account *url.URL, target interface{}) error {
		return batch.Account(account).Main().GetAs(target)
	}))

	// Change the name of the network
	if flagExport.NetworkName != "" {
		globals.Network.NetworkName = flagExport.NetworkName
	}

	// Overwrite the validator set
	if flagExport.Validators != "" {
		globals.Network.Validators = nil
		var val []*protocol.ValidatorInfo
		check(json.Unmarshal([]byte(flagExport.Validators), &val))
		for _, val := range val {
			for _, part := range val.Partitions {
				globals.Network.AddValidator(val.PublicKey, part.ID, part.Active)
			}
		}
	}

	// Take a snapshot
	header := new(snapshot.Header)
	header.Height = ledger.Index
	header.Timestamp = ledger.Timestamp

	buf := new(ioutil2.Buffer)
	w, err := snapshot.Collect(batch, header, buf, daemon.Logger.With("module", "snapshot"), func(account *database.Account) (bool, error) {
		return true, nil
	})
	check(err)
	check(snapshot.CollectAnchors(w, batch, &daemon.Config.Accumulate.Describe))

	// Build a new genesis doc
	doc := new(types.GenesisDoc)
	doc.InitialHeight = int64(ledger.Index) + 1
	doc.GenesisTime = ledger.Timestamp
	doc.ChainID = oldDoc.ChainID
	doc.ConsensusParams = oldDoc.ConsensusParams
	doc.AppHash = batch.BptRoot()
	doc.AppState, err = json.Marshal(buf.Bytes())
	check(err)

	// Apply the network ID prefix
	if flagExport.NetworkName != "" {
		doc.ChainID = flagExport.NetworkName + "-" + daemon.Config.Accumulate.PartitionId
	}

	// Build the validator list
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

	// Write the new genesis doc
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
