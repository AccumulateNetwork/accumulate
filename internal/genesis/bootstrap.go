package genesis

import (
	"fmt"
	"time"

	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/chain"
	"github.com/AccumulateNetwork/accumulate/internal/database"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/state"
	"github.com/tendermint/tendermint/libs/log"
	tmtypes "github.com/tendermint/tendermint/types"
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
		adi.ChainUrl = types.String(uAdi.String())
		adi.KeyBook = types.String(uBook.String())
		records = append(records, adi)

		book := protocol.NewKeyBook()
		book.ChainUrl = types.String(uBook.String())
		book.Pages = []string{uPage.String()}
		records = append(records, book)

		page := protocol.NewKeyPage()
		page.ChainUrl = types.String(uPage.String())
		page.KeyBook = types.String(uBook.String())
		records = append(records, page)

		page.Keys = make([]*protocol.KeySpec, len(opts.Validators))
		for i, val := range opts.Validators {
			spec := new(protocol.KeySpec)
			spec.PublicKey = val.PubKey.Bytes()
			page.Keys[i] = spec
		}

		// Create the ledger
		ledger := protocol.NewInternalLedger()
		ledger.ChainUrl = types.String(uAdi.JoinPath(protocol.Ledger).String())
		ledger.KeyBook = types.String(uBook.String())
		ledger.Synthetic.Nonce = 1
		records = append(records, ledger)

		// Create the anchor pool
		anchors := protocol.NewAnchor()
		anchors.ChainUrl = types.String(uAdi.JoinPath(protocol.AnchorPool).String())
		anchors.KeyBook = types.String(uBook.String())
		records = append(records, anchors)

		// Create records and directory entries
		urls := make([]*url.URL, len(records))
		for i, r := range records {
			urls[i], _ = r.Header().ParseUrl()
		}

		acme := new(protocol.TokenIssuer)
		acme.Type = types.ChainTypeTokenIssuer
		acme.KeyBook = types.String(uBook.String())
		acme.ChainUrl = types.String(protocol.AcmeUrl().String())
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
			lite.ChainUrl = types.String(protocol.FaucetWallet.Addr)
			lite.TokenUrl = protocol.AcmeUrl().String()
			lite.Balance.SetString("314159265358979323846264338327950288419716939937510582097494459", 10)
			records = append(records, lite)
		}

		st.Update(records...)
		return st.AddDirectoryEntry(urls...)
	})
}
