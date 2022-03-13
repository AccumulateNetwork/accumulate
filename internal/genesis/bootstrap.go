package genesis

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
	tmtypes "github.com/tendermint/tendermint/types"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/types/state"
)

type InitOpts struct {
	Network     config.Network
	Validators  []tmtypes.GenesisValidator
	GenesisTime time.Time
	Logger      log.Logger
}

func mustParseUrl(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

func Init(kvdb storage.KeyValueStore, opts InitOpts) ([]byte, error) {
	db := database.New(kvdb, opts.Logger.With("module", "database"))

	exec, err := chain.NewGenesisExecutor(db, opts.Logger, opts.Network)
	if err != nil {
		return nil, err
	}

	return exec.Genesis(opts.GenesisTime, func(st *chain.StateManager) error {
		var records []state.Chain

		// Create the ADI
		uAdi := opts.Network.NodeUrl()
		uBook := uAdi.JoinPath(protocol.ValidatorBook)
		uPage := uAdi.JoinPath(protocol.ValidatorBook + "0")

		adi := protocol.NewADI()
		adi.Url = uAdi
		adi.KeyBook = uBook
		records = append(records, adi)

		book := protocol.NewKeyBook()
		book.Url = uBook
		book.Pages = []*url.URL{uPage}
		records = append(records, book)

		page := protocol.NewKeyPage()
		page.Url = uPage
		page.KeyBook = uBook
		page.Threshold = protocol.GetValidatorsMOfN(len(opts.Validators))
		records = append(records, page)

		page.Keys = make([]*protocol.KeySpec, len(opts.Validators))
		for i, val := range opts.Validators {
			spec := new(protocol.KeySpec)
			spec.PublicKey = val.PubKey.Bytes()
			page.Keys[i] = spec
		}

		// set the initial price to 1/5 fct price * 1/4 market cap dilution = 1/20 fct price
		// for this exercise, we'll assume that 1 FCT = $1, so initial ACME price is $0.05
		oraclePrice := uint64(0.05 * protocol.AcmeOraclePrecision)

		// Create the ledger
		ledger := protocol.NewInternalLedger()
		ledger.Url = uAdi.JoinPath(protocol.Ledger)
		ledger.KeyBook = uBook
		ledger.Synthetic.Nonce = 1
		ledger.ActiveOracle = oraclePrice
		ledger.PendingOracle = ledger.ActiveOracle
		ledger.Index = protocol.GenesisBlock
		records = append(records, ledger)

		// Create the synth ledger
		synthLedger := protocol.NewInternalSyntheticLedger()
		synthLedger.Url = uAdi.JoinPath(protocol.SyntheticLedgerPath)
		synthLedger.KeyBook = uBook
		records = append(records, synthLedger)

		// Create the anchor pool
		anchors := protocol.NewAnchor()
		anchors.Url = uAdi.JoinPath(protocol.AnchorPool)
		anchors.KeyBook = uBook
		records = append(records, anchors)

		// Create records and directory entries
		urls := make([]*url.URL, len(records))
		for i, r := range records {
			urls[i], _ = r.Header().ParseUrl()
		}

		acme := new(protocol.TokenIssuer)
		acme.KeyBook = uBook
		acme.Url = protocol.AcmeUrl()
		acme.Precision = 8
		acme.Symbol = "ACME"
		records = append(records, acme)

		type DataRecord struct {
			Account *protocol.DataAccount
			Entry   *protocol.DataEntry
		}
		var dataRecords []DataRecord

		//create a vote scratch chain
		wd := new(protocol.WriteData)
		lci := types.LastCommitInfo{}
		wd.Entry.Data, err = json.Marshal(&lci)

		da := new(protocol.DataAccount)
		da.Scratch = true
		da.Url = uAdi.JoinPath(protocol.Votes)
		da.KeyBook = uBook

		records = append(records, da)
		urls = append(urls, da.Url)
		dataRecords = append(dataRecords, DataRecord{da, &wd.Entry})

		//create an evidence scratch chain
		da = new(protocol.DataAccount)
		da.Scratch = true
		da.Url = uAdi.JoinPath(protocol.Evidence)
		da.KeyBook = uBook

		records = append(records, da)
		urls = append(urls, da.Url)

		switch opts.Network.Type {
		case config.Directory:
			oracle := new(protocol.AcmeOracle)
			oracle.Price = oraclePrice
			wd := new(protocol.WriteData)
			wd.Entry.Data, err = json.Marshal(&oracle)

			da := new(protocol.DataAccount)
			da.Url = uAdi.JoinPath(protocol.Oracle)
			da.KeyBook = uBook

			records = append(records, da)
			urls = append(urls, da.Url)
			dataRecords = append(dataRecords, DataRecord{da, &wd.Entry})

			// TODO Move ACME to DN

		case config.BlockValidator:
			// Test with `${ID}` not `bvn-${ID}` because the latter will fail
			// with "bvn-${ID} is reserved"
			if err := protocol.IsValidAdiUrl(&url.URL{Authority: opts.Network.LocalSubnetID}); err != nil {
				panic(fmt.Errorf("%q is not a valid subnet ID: %v", opts.Network.LocalSubnetID, err))
			}

			lite := protocol.NewLiteTokenAccount()
			lite.Url = protocol.FaucetUrl
			lite.TokenUrl = protocol.AcmeUrl()
			lite.Balance.SetString("314159265358979323846264338327950288419716939937510582097494459", 10)
			records = append(records, lite)
		}

		st.Update(records...)

		for _, wd := range dataRecords {
			st.UpdateData(wd.Account, wd.Entry.Hash(), wd.Entry)
		}

		adiUrl, _ := adi.Header().ParseUrl()
		return st.AddDirectoryEntry(adiUrl, urls...)
	})
}
