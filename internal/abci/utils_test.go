package abci_test

import (
	"context"
	"crypto/ed25519"
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/abci"
	accapi "github.com/AccumulateNetwork/accumulate/internal/api"
	api2 "github.com/AccumulateNetwork/accumulate/internal/api/v2"
	"github.com/AccumulateNetwork/accumulate/internal/chain"
	"github.com/AccumulateNetwork/accumulate/internal/database"
	"github.com/AccumulateNetwork/accumulate/internal/genesis"
	"github.com/AccumulateNetwork/accumulate/internal/logging"
	"github.com/AccumulateNetwork/accumulate/internal/relay"
	acctesting "github.com/AccumulateNetwork/accumulate/internal/testing"
	"github.com/AccumulateNetwork/accumulate/internal/testing/e2e"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/smt/storage/memory"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api"
	"github.com/AccumulateNetwork/accumulate/types/api/query"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	abcitypes "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/crypto"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	tmtypes "github.com/tendermint/tendermint/types"
)

const logConsole = true

var reAlphaNum = regexp.MustCompile("[^a-zA-Z0-9]")

func createAppWithMemDB(t testing.TB, addr crypto.Address, doGenesis bool) *fakeNode {
	return createApp(t, nil, addr, doGenesis)
}

func createApp(t testing.TB, db *database.Database, addr crypto.Address, doGenesis bool) *fakeNode {
	n := new(fakeNode)
	n.t = t
	_, n.key, _ = ed25519.GenerateKey(rand)

	subnet := reAlphaNum.ReplaceAllString(t.Name(), "-")
	n.network = &config.Network{
		Type:     config.BlockValidator,
		ID:       subnet,
		BvnNames: []string{subnet},
	}

	var logWriter io.Writer
	var err error
	if logConsole {
		logWriter, err = logging.NewConsoleWriter("plain")
	} else {
		logWriter, err = logging.TestLogWriter(t)("plain")
	}
	require.NoError(t, err)
	logLevel, logWriter, err := logging.ParseLogLevel(config.DefaultLogLevels, logWriter)
	require.NoError(t, err)
	logger, err := logging.NewTendermintLogger(zerolog.New(logWriter), logLevel, false)
	require.NoError(t, err)

	if db == nil {
		db, err = database.Open("", true, logger)
		require.NoError(t, err)
	}
	n.db = db

	appChan := make(chan abcitypes.Application)
	defer close(appChan)

	batch := db.Begin()
	defer batch.Discard()

	ledger := protocol.NewInternalLedger()
	err = batch.Account(n.network.NodeUrl(protocol.Ledger)).GetStateAs(ledger)
	if err == nil {
		n.height = ledger.Index
	} else {
		require.ErrorIs(t, err, storage.ErrNotFound)
	}

	n.client = acctesting.NewFakeTendermint(appChan, db, n.network, n.NextHeight, func(err error) {
		t.Helper()
		assert.NoError(t, err)
	}, 100*time.Millisecond)
	relay := relay.New(n.client)
	require.NoError(t, relay.Start())
	t.Cleanup(func() { require.NoError(t, relay.Stop()) })
	n.query = accapi.NewQuery(relay)

	mgr, err := chain.NewNodeExecutor(chain.ExecutorOptions{
		Local:   n.client,
		DB:      n.db,
		IsTest:  true,
		Logger:  logger,
		Key:     n.key,
		Network: *n.network,
	})
	require.NoError(t, err)

	n.app = abci.NewAccumulator(abci.AccumulatorOptions{
		Chain:   mgr,
		DB:      db,
		Logger:  logger,
		Network: *n.network,
		Address: addr,
	})
	appChan <- n.app
	n.app.(*abci.Accumulator).OnFatal(func(err error) {
		require.NoError(t, err)
	})

	require.NoError(t, mgr.Start())
	t.Cleanup(func() { _ = mgr.Stop() })
	t.Cleanup(func() { n.client.Shutdown() })

	if !doGenesis {
		return n
	}

	n.height++

	kv := new(memory.DB)
	_ = kv.InitDB("", nil)
	_, err = genesis.Init(kv, genesis.InitOpts{
		Network:     *n.network,
		GenesisTime: time.Now(),
		Logger:      logger,
		Validators: []tmtypes.GenesisValidator{
			{PubKey: tmed25519.PrivKey(n.key).PubKey()},
		},
	})
	require.NoError(t, err)

	state, err := kv.MarshalJSON()
	require.NoError(t, err)

	n.app.InitChain(abcitypes.RequestInitChain{
		Time:          time.Now(),
		ChainId:       subnet,
		AppStateBytes: state,
	})

	return n
}

type fakeNode struct {
	t       testing.TB
	db      *database.Database
	network *config.Network
	app     abcitypes.Application
	client  *acctesting.FakeTendermint
	query   *accapi.Query
	key     ed25519.PrivateKey
	height  int64
}

func (n *fakeNode) NextHeight() int64 {
	n.height++
	return n.height
}

func (n *fakeNode) Query(q *query.Query) *api.APIDataResponse {
	payload, err := q.MarshalBinary()
	require.NoError(n.t, err)

	resp := n.app.Query(abcitypes.RequestQuery{Data: payload})
	require.Zero(n.t, resp.Code, "Query failed: %s", resp.Info)

	var msg json.RawMessage = []byte(fmt.Sprintf("{\"entry\":\"%x\"}", resp.Value))
	chain := new(state.ChainHeader)
	require.NoError(n.t, chain.UnmarshalBinary(resp.Value))
	return &api.APIDataResponse{Type: types.String(chain.Type.Name()), Data: &msg}
}

func (n *fakeNode) GetChainStateByUrl(url string) *api.APIDataResponse {
	r, err := n.query.GetChainStateByUrl(url)
	require.NoError(n.t, err)
	return r
}

func (n *fakeNode) GetChainDataByUrl(url string) *api.APIDataResponse {
	r, err := n.query.QueryDataByUrl(url)
	require.NoError(n.t, err)
	return r
}

func (n *fakeNode) GetChainDataByEntryHash(url string, entryHash []byte) *api.APIDataResponse {
	r, err := n.query.GetDataByEntryHash(url, entryHash)
	require.NoError(n.t, err)
	return r
}

func (n *fakeNode) GetChainDataSet(url string, start uint64, limit uint64, expand bool) *api.APIDataResponsePagination {
	r, err := n.query.GetDataSetByUrl(url, start, limit, expand)
	require.NoError(n.t, err)
	return r
}

func (n *fakeNode) GetChainStateByTxId(txid []byte) *api.APIDataResponse {
	r, err := n.query.GetChainStateByTxId(txid)
	require.NoError(n.t, err)
	return r
}

func (n *fakeNode) GetChainStateByChainId(txid []byte) *api.APIDataResponse {
	r, err := n.query.GetChainStateByChainId(txid)
	require.NoError(n.t, err)
	return r
}

func (n *fakeNode) Batch(inBlock func(func(*transactions.Envelope))) [][32]byte {
	n.t.Helper()

	var ids [][32]byte
	var blob []byte
	inBlock(func(tx *transactions.Envelope) {
		var id [32]byte
		copy(id[:], tx.Transaction.Hash())
		ids = append(ids, id)
		b, err := tx.MarshalBinary()
		require.NoError(n.t, err)
		blob = append(blob, b...)
	})

	// Submit all the transactions as a batch
	n.client.SubmitTx(context.Background(), blob)

	// n.client.WaitForAll()
	for _, id := range ids {
		n.client.WaitFor(id, true)
	}
	return ids
}

func generateKey() tmed25519.PrivKey {
	_, key, _ := ed25519.GenerateKey(rand)
	return tmed25519.PrivKey(key)
}

func edSigner(key tmed25519.PrivKey, nonce uint64) func(hash []byte) (*transactions.ED25519Sig, error) {
	return func(hash []byte) (*transactions.ED25519Sig, error) {
		sig := new(transactions.ED25519Sig)
		return sig, sig.Sign(nonce, key, hash)
	}
}

func (n *fakeNode) ParseUrl(s string) *url.URL {
	u, err := url.Parse(s)
	require.NoError(n.t, err)
	return u
}

func (n *fakeNode) GetDirectory(adi string) []string {
	batch := n.db.Begin()
	defer batch.Discard()

	u := n.ParseUrl(adi)
	record := batch.Account(u)
	require.True(n.t, u.Identity().Equal(u))

	md := new(protocol.DirectoryIndexMetadata)
	err := record.Index("Directory", "Metadata").GetAs(md)
	if errors.Is(err, storage.ErrNotFound) {
		return nil
	}
	require.NoError(n.t, err)

	chains := make([]string, md.Count)
	for i := range chains {
		data, err := record.Index("Directory", uint64(i)).Get()
		require.NoError(n.t, err)
		chains[i] = string(data)
	}
	return chains
}

func (n *fakeNode) GetTx(txid []byte) *api2.TransactionQueryResponse {
	q := api2.NewQueryDirect(n.client, api2.QuerierOptions{})
	resp, err := q.QueryTx(txid, 0)
	require.NoError(n.t, err)
	data, err := json.Marshal(resp.Data)
	require.NoError(n.t, err)

	var typ types.TransactionType
	require.NoError(n.t, typ.UnmarshalJSON([]byte(strconv.Quote(resp.Type))))

	resp.Data, err = protocol.NewTransaction(typ)
	require.NoError(n.t, err)
	require.NoError(n.t, json.Unmarshal(data, resp.Data))

	return resp
}

func (n *fakeNode) GetChainAs(url string, obj encoding.BinaryUnmarshaler) {
	r, err := n.query.QueryByUrl(url)
	require.NoError(n.t, err)

	if r.Response.Code != 0 {
		n.t.Fatalf("query for %q failed with code %d: %s", url, r.Response.Code, r.Response.Info)
	}

	so := state.Object{}
	err = so.UnmarshalBinary(r.Response.Value)
	if err != nil {
		n.t.Fatalf("error unmarshaling state object %v", err)
	}

	require.NoError(n.t, obj.UnmarshalBinary(so.Entry))
}

func (n *fakeNode) GetDataAccount(url string) *protocol.DataAccount {
	acct := protocol.NewDataAccount()
	n.GetChainAs(url, acct)
	return acct
}

func (n *fakeNode) GetTokenAccount(url string) *protocol.TokenAccount {
	acct := protocol.NewTokenAccount()
	n.GetChainAs(url, acct)
	return acct
}

func (n *fakeNode) GetLiteTokenAccount(url string) *protocol.LiteTokenAccount {
	acct := new(protocol.LiteTokenAccount)
	n.GetChainAs(url, acct)
	return acct
}

func (n *fakeNode) GetADI(url string) *protocol.ADI {
	adi := new(protocol.ADI)
	n.GetChainAs(url, adi)
	return adi
}

func (n *fakeNode) GetKeyBook(url string) *protocol.KeyBook {
	book := new(protocol.KeyBook)
	n.GetChainAs(url, book)
	return book
}

func (n *fakeNode) GetKeyPage(url string) *protocol.KeyPage {
	mss := new(protocol.KeyPage)
	n.GetChainAs(url, mss)
	return mss
}

func (n *fakeNode) GetTokenIssuer(url string) *protocol.TokenIssuer {
	mss := new(protocol.TokenIssuer)
	n.GetChainAs(url, mss)
	return mss
}

type e2eDUT struct {
	*e2e.Suite
	*fakeNode
}

func (d *e2eDUT) getObj(url string) *state.Object {
	r, err := d.query.QueryByUrl(url)
	d.Require().NoError(err)
	d.Require().Zero(r.Response.Code, "Query failed: %v", r.Response.Info)

	obj := new(state.Object)
	d.Require().Equal([]byte("chain"), r.Response.Key)
	d.Require().NoError(obj.UnmarshalBinary(r.Response.Value))
	return obj
}

func (d *e2eDUT) GetRecordAs(url string, target state.Chain) {
	d.Require().NoError(d.getObj(url).As(target))
}

func (d *e2eDUT) GetRecordHeight(url string) uint64 {
	return d.getObj(url).Height
}

func (d *e2eDUT) SubmitTxn(tx *transactions.Envelope) {
	b, err := tx.MarshalBinary()
	d.Require().NoError(err)
	d.client.SubmitTx(context.Background(), b)
}

func (d *e2eDUT) WaitForTxns(ids ...[]byte) {
	for _, id := range ids {
		var id32 [32]byte
		copy(id32[:], id)
		d.client.WaitFor(id32, true)
	}
}
