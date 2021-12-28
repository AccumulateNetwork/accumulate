package genesis

import (
	"fmt"
	"time"

	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/chain"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/state"
	tmtypes "github.com/tendermint/tendermint/types"
)

type InitOpts struct {
	SubnetID    string
	NetworkType config.NetworkType
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

func Init(kvdb storage.KeyValueDB, opts InitOpts) ([]byte, error) {
	db := new(state.StateDB)
	err := db.Load(kvdb, false)
	if err != nil {
		return nil, err
	}

	_ = kvdb.Put(storage.MakeKey("SubnetID"), []byte(opts.SubnetID))

	exec, err := chain.NewGenesisExecutor(db, opts.NetworkType)
	if err != nil {
		return nil, err
	}

	return exec.Genesis(opts.GenesisTime, func(st *chain.StateManager) error {
		acme := new(protocol.TokenIssuer)
		acme.Type = types.ChainTypeTokenIssuer
		acme.ChainUrl = types.String(protocol.AcmeUrl().String())
		acme.Precision = 8
		acme.Symbol = "ACME"
		st.Update(acme)

		var uAdi *url.URL
		switch opts.NetworkType {
		case config.Directory:
			uAdi = protocol.DnUrl()

		case config.BlockValidator:
			// Test with `${ID}` not `bvn-${ID}` because the latter will fail
			// with "bvn-${ID} is reserved"
			if err := protocol.IsValidAdiUrl(&url.URL{Authority: opts.SubnetID}); err != nil {
				panic(fmt.Errorf("%q is not a valid subnet ID: %v", opts.SubnetID, err))
			}
			uAdi = protocol.BvnUrl(opts.SubnetID)

			lite := protocol.NewLiteTokenAccount()
			lite.ChainUrl = types.String(protocol.FaucetWallet.Addr)
			lite.TokenUrl = protocol.AcmeUrl().String()
			lite.Balance.SetString("314159265358979323846264338327950288419716939937510582097494459", 10)
			st.Update(lite)
		}

		// Create the ADI
		uBook := uAdi.JoinPath("validators")
		uPage := uAdi.JoinPath("validators0")

		adi := state.NewIdentityState(types.String(uAdi.String()))
		adi.KeyBook = types.String(uBook.String())

		book := protocol.NewKeyBook()
		book.ChainUrl = types.String(uBook.String())
		book.Pages = []string{uPage.String()}

		page := protocol.NewKeyPage()
		page.ChainUrl = types.String(uPage.String())
		page.KeyBook = types.String(uBook.String())

		page.Keys = make([]*protocol.KeySpec, len(opts.Validators))
		for i, val := range opts.Validators {
			spec := new(protocol.KeySpec)
			spec.PublicKey = val.PubKey.Bytes()
			page.Keys[i] = spec
		}

		st.Update(adi, book, page)
		return st.AddDirectoryEntry(uAdi, uBook, uPage)
	})
}
