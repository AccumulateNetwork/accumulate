package genesis

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/chain"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
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

	_ = kvdb.Put(storage.ComputeKey("SubnetID"), []byte(opts.SubnetID))

	tx := new(transactions.GenTransaction)
	tx.SigInfo = new(transactions.SignatureInfo)
	tx.SigInfo.URL = protocol.ACME
	tx.Transaction, err = new(protocol.SyntheticGenesis).MarshalBinary()
	if err != nil {
		return nil, err
	}

	dbtx := db.Begin()
	st, err := chain.NewStateManager(dbtx, tx)
	if err == nil {
		return nil, errors.New("already initialized")
	} else if !errors.Is(err, storage.ErrNotFound) {
		return nil, err
	}

	txPending := state.NewPendingTransaction(tx)
	txAccepted, txPending := state.NewTransaction(txPending)
	dataAccepted, err := txAccepted.MarshalBinary()
	if err != nil {
		return nil, err
	}
	txPending.Status = json.RawMessage(fmt.Sprintf("{\"code\":\"0\"}"))
	dataPending, err := txPending.MarshalBinary()
	if err != nil {
		return nil, err
	}

	chainId := protocol.AcmeUrl().ResourceChain32()
	err = dbtx.AddTransaction((*types.Bytes32)(&chainId), tx.TransactionHash(), &state.Object{Entry: dataPending}, &state.Object{Entry: dataAccepted})
	if err != nil {
		return nil, &protocol.Error{Code: protocol.CodeTxnStateError, Message: err}
	}

	acme := new(protocol.TokenIssuer)
	acme.Type = types.ChainTypeTokenIssuer
	acme.ChainUrl = types.String(protocol.AcmeUrl().String())
	acme.Precision = 8
	acme.Symbol = "ACME"
	st.Update(acme)

	var uAdi *url.URL
	switch opts.NetworkType {
	case config.Directory:
		uAdi = mustParseUrl("dn")

	case config.BlockValidator:
		uAdi = mustParseUrl("bvn-" + opts.SubnetID)

		anon := protocol.NewLiteTokenAccount()
		anon.ChainUrl = types.String(protocol.FaucetWallet.Addr)
		anon.TokenUrl = protocol.AcmeUrl().String()
		anon.Balance.SetString("314159265358979323846264338327950288419716939937510582097494459", 10)
		st.Update(anon)
	}

	// Create the ADI
	uBook := uAdi.JoinPath("validators")
	uPage := uAdi.JoinPath("validators0")

	adi := state.NewIdentityState(types.String(uAdi.String()))
	adi.SigSpecId = uBook.ResourceChain32()

	book := protocol.NewSigSpecGroup()
	book.ChainUrl = types.String(uBook.String())
	book.SigSpecs = [][32]byte{uPage.ResourceChain32()}

	page := protocol.NewSigSpec()
	page.ChainUrl = types.String(uPage.String())
	page.SigSpecId = uBook.ResourceChain32()

	page.Keys = make([]*protocol.KeySpec, len(opts.Validators))
	for i, val := range opts.Validators {
		spec := new(protocol.KeySpec)
		spec.PublicKey = val.PubKey.Bytes()
		page.Keys[i] = spec
	}

	st.Update(adi, book, page)

	// Commit the changes
	err = st.Commit()
	if err != nil {
		return nil, err
	}
	return dbtx.Commit(1, opts.GenesisTime)
}
