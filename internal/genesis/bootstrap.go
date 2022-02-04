package genesis

import (
	"fmt"
	"time"

	"github.com/tendermint/tendermint/libs/log"
	tmtypes "github.com/tendermint/tendermint/types"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/types"
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
		adi.Url = uAdi.String()
		adi.KeyBook = uBook.String()
		records = append(records, adi)

		book := protocol.NewKeyBook()
		book.Url = uBook.String()
		book.Pages = []string{uPage.String()}
		records = append(records, book)

		page := protocol.NewKeyPage()
		page.Url = uPage.String()
		page.KeyBook = uBook.String()
		page.Threshold = 1
		records = append(records, page)

		page.Keys = make([]*protocol.KeySpec, len(opts.Validators))
		for i, val := range opts.Validators {
			spec := new(protocol.KeySpec)
			spec.PublicKey = val.PubKey.Bytes()
			page.Keys[i] = spec
		}

		// Create the ledger
		ledger := protocol.NewInternalLedger()
		ledger.Url = uAdi.JoinPath(protocol.Ledger).String()
		ledger.KeyBook = uBook.String()
		ledger.Synthetic.Nonce = 1
		records = append(records, ledger)

		// Create the anchor pool
		anchors := protocol.NewAnchor()
		anchors.Url = uAdi.JoinPath(protocol.AnchorPool).String()
		anchors.KeyBook = uBook.String()
		records = append(records, anchors)

		// Create records and directory entries
		urls := make([]*url.URL, len(records))
		for i, r := range records {
			urls[i], _ = r.Header().ParseUrl()
		}

		acme := new(protocol.TokenIssuer)
		acme.Type = types.AccountTypeTokenIssuer
		acme.KeyBook = uBook.String()
		acme.Url = protocol.AcmeUrl().String()
		acme.Precision = 8
		acme.Symbol = "ACME"
		records = append(records, acme)

		switch opts.Network.Type {
		case config.Directory:
			// TODO Move ACME to DN

		case config.BlockValidator:
			// Test with `${ID}` not `bvn-${ID}` because the latter will fail
			// with "bvn-${ID} is reserved"
			if err := protocol.IsValidAdiUrl(&url.URL{Authority: opts.Network.ID}); err != nil {
				panic(fmt.Errorf("%q is not a valid subnet ID: %v", opts.Network.ID, err))
			}

			lite := protocol.NewLiteTokenAccount()
			lite.Url = protocol.FaucetUrl.String()
			lite.TokenUrl = protocol.AcmeUrl().String()
			lite.Balance.SetString("314159265358979323846264338327950288419716939937510582097494459", 10)
			records = append(records, lite)
		}

		st.Update(records...)
		return st.AddDirectoryEntry(urls...)
	})
}
