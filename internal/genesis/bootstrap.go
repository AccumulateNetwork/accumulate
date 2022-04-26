package genesis

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
	tmtypes "github.com/tendermint/tendermint/types"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

type InitOpts struct {
	Network     config.Network
	Validators  []tmtypes.GenesisValidator
	GenesisTime time.Time
	Logger      log.Logger
}

func Init(kvdb storage.KeyValueStore, opts InitOpts) ([]byte, error) {
	db := database.New(kvdb, opts.Logger.With("module", "database"))

	exec, err := block.NewGenesisExecutor(db, opts.Logger, opts.Network)
	if err != nil {
		return nil, err
	}

	block := new(block.Block)
	block.Index = protocol.GenesisBlock
	block.Time = opts.GenesisTime
	block.Batch = db.Begin(true)
	defer block.Batch.Discard()

	err = exec.Genesis(block, func(st *chain.StateManager) error {
		var records []protocol.Account

		// Create the ADI
		uAdi := opts.Network.NodeUrl()
		uBook := uAdi.JoinPath(protocol.ValidatorBook)

		adi := new(protocol.ADI)
		adi.Url = uAdi
		adi.AddAuthority(uBook)
		records = append(records, adi)

		book := new(protocol.KeyBook)
		book.Url = uBook
		book.AddAuthority(uBook)
		book.PageCount = 1
		records = append(records, book)

		page := new(protocol.KeyPage)
		page.Url = protocol.FormatKeyPageUrl(uBook, 0)

		page.AcceptThreshold = protocol.GetValidatorsMOfN(len(opts.Validators), nil)
		page.Version = 1
		records = append(records, page)

		page.Keys = make([]*protocol.KeySpec, len(opts.Validators))
		for i, val := range opts.Validators {
			spec := new(protocol.KeySpec)
			kh := sha256.Sum256(val.PubKey.Bytes())
			spec.PublicKeyHash = kh[:]
			page.Keys[i] = spec
		}

		// set the initial price to 1/5 fct price * 1/4 market cap dilution = 1/20 fct price
		// for this exercise, we'll assume that 1 FCT = $1, so initial ACME price is $0.05
		oraclePrice := uint64(protocol.InitialAcmeOracleValue)

		// Create the ledger
		ledger := new(protocol.InternalLedger)
		ledger.Url = uAdi.JoinPath(protocol.Ledger)
		ledger.AddAuthority(uBook)
		ledger.Synthetic.Nonce = 1
		ledger.ActiveOracle = oraclePrice
		ledger.PendingOracle = ledger.ActiveOracle
		ledger.Index = protocol.GenesisBlock
		records = append(records, ledger)

		// Create the anchor pool
		anchors := new(protocol.Anchor)
		anchors.Url = uAdi.JoinPath(protocol.AnchorPool)
		anchors.AddAuthority(uBook)
		records = append(records, anchors)

		// Create records and directory entries
		urls := make([]*url.URL, len(records))
		for i, r := range records {
			urls[i] = r.GetUrl()
		}

		type DataRecord struct {
			Account *protocol.DataAccount
			Entry   *protocol.DataEntry
		}
		var dataRecords []DataRecord

		//create a vote scratch chain
		wd := new(protocol.WriteData)
		lci := types.LastCommitInfo{}
		d, err := json.Marshal(&lci)
		if err != nil {
			return err
		}
		wd.Entry.Data = append(wd.Entry.Data, d)

		da := new(protocol.DataAccount)
		da.Scratch = true
		da.Url = uAdi.JoinPath(protocol.Votes)
		da.AddAuthority(uBook)

		records = append(records, da)
		urls = append(urls, da.Url)
		dataRecords = append(dataRecords, DataRecord{da, &wd.Entry})

		//create an evidence scratch chain
		da = new(protocol.DataAccount)
		da.Scratch = true
		da.Url = uAdi.JoinPath(protocol.Evidence)
		da.AddAuthority(uBook)

		records = append(records, da)
		urls = append(urls, da.Url)

		//create a new Globals account
		global := new(protocol.DataAccount)
		global.Url = uAdi.JoinPath(protocol.Globals)
		wg := new(protocol.WriteData)
		threshold := new(protocol.NetworkGlobals)
		threshold.ValidatorThreshold.Numerator = 2
		threshold.ValidatorThreshold.Denominator = 3
		var dat []byte
		dat, err = threshold.MarshalBinary()
		if err != nil {
			return err
		}
		wg.Entry.Data = append(wg.Entry.Data, dat)
		global.AddAuthority(uBook)
		records = append(records, global)
		urls = append(urls, global.Url)
		dataRecords = append(dataRecords, DataRecord{global, &wg.Entry})

		switch opts.Network.Type {
		case config.Directory:
			oracle := new(protocol.AcmeOracle)
			oracle.Price = oraclePrice
			wd := new(protocol.WriteData)
			d, err = json.Marshal(&oracle)
			if err != nil {
				return err
			}
			wd.Entry.Data = append(wd.Entry.Data, d)
			da := new(protocol.DataAccount)
			da.Url = uAdi.JoinPath(protocol.Oracle)
			da.AddAuthority(uBook)

			records = append(records, da)
			urls = append(urls, da.Url)
			dataRecords = append(dataRecords, DataRecord{da, &wd.Entry})

			acme := new(protocol.TokenIssuer)
			acme.AddAuthority(uBook)
			acme.Url = protocol.AcmeUrl()
			acme.Precision = 8
			acme.Symbol = "ACME"
			records = append(records, acme)

			if protocol.IsTestNet {
				// On the TestNet, set the issued amount to the faucet balance
				acme.Issued.SetString(protocol.AcmeFaucetBalance, 10)
			} else {
				// On the MainNet, set the supply limit
				acme.SupplyLimit = big.NewInt(protocol.AcmeSupplyLimit * protocol.AcmePrecision)
			}

		case config.BlockValidator:
			// Test with `${ID}` not `bvn-${ID}` because the latter will fail
			// with "bvn-${ID} is reserved"
			if err := protocol.IsValidAdiUrl(&url.URL{Authority: opts.Network.LocalSubnetID}); err != nil {
				panic(fmt.Errorf("%q is not a valid subnet ID: %v", opts.Network.LocalSubnetID, err))
			}

			subnet, err := routing.RouteAccount(&opts.Network, protocol.FaucetUrl)
			if err == nil && subnet == opts.Network.LocalSubnetID {
				lite := new(protocol.LiteTokenAccount)
				lite.Url = protocol.FaucetUrl
				lite.TokenUrl = protocol.AcmeUrl()
				lite.Balance.SetString(protocol.AcmeFaucetBalance, 10)
				records = append(records, lite)
			}
		}

		st.Update(records...)

		for _, wd := range dataRecords {
			st.UpdateData(wd.Account, wd.Entry.Hash(), wd.Entry)
		}

		return st.AddDirectoryEntry(adi.Url, urls...)
	})
	if err != nil {
		return nil, err
	}

	err = block.Batch.Commit()
	if err != nil {
		return nil, err
	}

	batch := db.Begin(false)
	defer batch.Discard()
	return batch.GetMinorRootChainAnchor(&opts.Network)
}
