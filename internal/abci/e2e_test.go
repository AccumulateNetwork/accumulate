package abci_test

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/AccumulateNetwork/accumulated/internal/abci"
	accapi "github.com/AccumulateNetwork/accumulated/internal/api"
	"github.com/AccumulateNetwork/accumulated/internal/chain"
	"github.com/AccumulateNetwork/accumulated/internal/relay"
	"github.com/AccumulateNetwork/accumulated/types"
	anon "github.com/AccumulateNetwork/accumulated/types/anonaddress"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"
	"github.com/stretchr/testify/require"
	abcitypes "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/crypto"
	tmtypes "github.com/tendermint/tendermint/proto/tendermint/types"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	randpkg "golang.org/x/exp/rand"
)

var rand = randpkg.New(randpkg.NewSource(0))
var fakeTxid = sha256.Sum256([]byte("fake txid"))

type Tx = transactions.GenTransaction

func TestE2E_Accumulator_AnonToken(t *testing.T) {
	app := createApp(t)

	_, sponsor, _ := ed25519.GenerateKey(rand)
	_, recipient, _ := ed25519.GenerateKey(rand)

	appSendTx(t, app, func(send func(*Tx)) {
		send(createFakeSyntheticDeposit(t, sponsor, recipient))
	})

	origin := transactions.NewWalletEntry()
	origin.Nonce = 1
	origin.PrivateKey = recipient
	origin.Addr = anon.GenerateAcmeAddress(recipient.Public().(ed25519.PublicKey))

	recipients := make([]*transactions.WalletEntry, 10)
	for i := range recipients {
		recipients[i] = transactions.NewWalletEntry()
	}

	appSendTx(t, app, func(send func(*Tx)) {
		for i := 0; i < 10; i++ {
			recipient := recipients[rand.Intn(len(recipients))]
			output := transactions.Output{Dest: recipient.Addr, Amount: 1000}
			exch := transactions.NewTokenSend(origin.Addr, output)
			tx, err := transactions.New(origin, exch)
			require.NoError(t, err)
			send(tx)
		}
	})

	t.Log(mustJSON(t, appGetChainState(t, app, origin.Addr+"/dc/ACME")))
}

func BenchmarkE2E_Accumulator_AnonToken(b *testing.B) {
	app := createApp(b)

	_, sponsor, _ := ed25519.GenerateKey(rand)
	_, recipient, _ := ed25519.GenerateKey(rand)

	appSendTx(b, app, func(send func(*Tx)) {
		send(createFakeSyntheticDeposit(b, sponsor, recipient))
	})

	origin := transactions.NewWalletEntry()
	origin.Nonce = 1
	origin.PrivateKey = recipient
	origin.Addr = anon.GenerateAcmeAddress(recipient.Public().(ed25519.PublicKey))

	rwallet := transactions.NewWalletEntry()

	b.ResetTimer()
	appSendTx(b, app, func(send func(*Tx)) {
		for i := 0; i < b.N; i++ {
			output := transactions.Output{Dest: rwallet.Addr, Amount: 1000}
			exch := transactions.NewTokenSend(origin.Addr, output)
			tx, err := transactions.New(origin, exch)
			require.NoError(b, err)
			send(tx)
		}
	})
}

func createApp(t testing.TB) abcitypes.Application {
	_, bvcKey, _ := ed25519.GenerateKey(rand)

	appId := sha256.Sum256([]byte("foo bar"))
	db := new(state.StateDB)
	err := db.Open("valacc.db", appId[:], true, true)
	require.NoError(t, err)

	rpcClient, err := rpchttp.New("tcp://foo.bar:123")
	require.NoError(t, err)

	bvc := chain.NewBlockValidator()
	mgr, err := chain.NewManager(accapi.NewQuery(relay.New(rpcClient)), db, bvcKey, bvc)
	require.NoError(t, err)

	app, err := abci.NewAccumulator(db, crypto.Address{}, mgr)
	require.NoError(t, err)

	return app
}

func mustJSON(t testing.TB, v interface{}) string {
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

func createFakeSyntheticDeposit(t testing.TB, sponsor, recipient ed25519.PrivateKey) *Tx {
	t.Helper()

	sponsorAdi := types.String(anon.GenerateAcmeAddress(sponsor.Public().(ed25519.PublicKey)))
	recipientAdi := types.String(anon.GenerateAcmeAddress(recipient.Public().(ed25519.PublicKey)))

	//create a fake synthetic deposit for faucet.
	deposit := synthetic.NewTokenTransactionDeposit(fakeTxid[:], &sponsorAdi, &recipientAdi)
	amtToDeposit := int64(50000)                             //deposit 50k tokens
	deposit.DepositAmount.SetInt64(amtToDeposit * 100000000) // assume 8 decimal places
	deposit.TokenUrl = types.String("dc/ACME")

	depData, err := deposit.MarshalBinary()
	require.NoError(t, err)

	tx := new(Tx)
	tx.SigInfo = new(transactions.SignatureInfo)
	tx.Transaction = depData
	tx.SigInfo.URL = *recipientAdi.AsString()
	tx.ChainID = types.GetChainIdFromChainPath(recipientAdi.AsString())[:]
	tx.Routing = types.GetAddressFromIdentity(recipientAdi.AsString())

	ed := new(transactions.ED25519Sig)
	tx.SigInfo.Nonce = 1
	ed.PublicKey = recipient.Public().(ed25519.PublicKey)
	err = ed.Sign(tx.SigInfo.Nonce, recipient, tx.TransactionHash())
	require.NoError(t, err)
	tx.Signature = append(tx.Signature, ed)
	return tx
}

func appQuery(t testing.TB, app abcitypes.Application, q *api.Query) *api.APIDataResponse {
	payload, err := q.MarshalBinary()
	require.NoError(t, err)

	resp := app.Query(abcitypes.RequestQuery{Data: payload})
	require.Zero(t, resp.Code)

	var msg json.RawMessage = []byte(fmt.Sprintf("{\"entry\":\"%x\"}", resp.Value))
	chain := new(state.Chain)
	require.NoError(t, chain.UnmarshalBinary(resp.Value))
	return &api.APIDataResponse{Type: types.String(chain.Type.Name()), Data: &msg}
}

func appGetChainState(t testing.TB, app abcitypes.Application, url string) *api.APIDataResponse {
	q := new(api.Query)
	q.Url = url
	q.RouteId = types.GetAddressFromIdentity(&url)
	q.ChainId = types.GetChainIdFromChainPath(&url).Bytes()
	return appQuery(t, app, q)
}

var lastHeight int64

func appSendTx(t testing.TB, app abcitypes.Application, do func(func(*Tx))) {
	t.Helper()

	lastHeight++
	app.BeginBlock(abcitypes.RequestBeginBlock{Header: tmtypes.Header{Height: lastHeight, Time: time.Now()}})

	do(func(gtx *Tx) {
		tx, err := gtx.Marshal()
		require.NoError(t, err)

		cresp := app.CheckTx(abcitypes.RequestCheckTx{Tx: tx})
		require.Zero(t, cresp.Code, cresp.Info)
		dresp := app.DeliverTx(abcitypes.RequestDeliverTx{Tx: tx})
		require.Zero(t, dresp.Code, dresp.Info)
	})

	app.EndBlock(abcitypes.RequestEndBlock{})

	app.Commit()
}
