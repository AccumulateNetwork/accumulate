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
	tmtypes "github.com/tendermint/tendermint/types"
)

type InitOpts struct {
	Network     config.Network
	Validators  []tmtypes.GenesisValidator
	GenesisTime time.Time
}

func mustParseUrl(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

func Init(kvdb storage.KeyValueStore, opts InitOpts) ([]byte, error) {
	db := database.New(kvdb, nil)

	exec, err := chain.NewGenesisExecutor(db, opts.Network)
	if err != nil {
		return nil, err
	}

	return exec.Genesis(opts.GenesisTime, func(st *chain.StateManager) error {
		var records []state.Chain

		// Create the ADI
		uAdi := opts.Network.NodeUrl()
		uBook := uAdi.JoinPath("validators")
		uPage := uAdi.JoinPath("validators0")

		adi := state.NewIdentityState(types.String(uAdi.String()))
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

		// Create the root chains
		majorRoot, minorRoot := state.NewAnchor(), state.NewAnchor()
		majorRoot.ChainUrl = types.String(uAdi.JoinPath(protocol.MajorRoot).String())
		minorRoot.ChainUrl = types.String(uAdi.JoinPath(protocol.MinorRoot).String())
		majorRoot.KeyBook = types.String(uBook.String())
		minorRoot.KeyBook = types.String(uBook.String())
		records = append(records, majorRoot, minorRoot)

		// Create the synthetic transaction chain
		synthTxn := state.NewSyntheticTransactionChain()
		synthTxn.ChainUrl = types.String(uAdi.JoinPath(protocol.Synthetic).String())
		synthTxn.KeyBook = types.String(uBook.String())
		records = append(records, synthTxn)

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
