package abci_test

import (
	"context"
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"testing"
	"time"

	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/abci"
	"github.com/AccumulateNetwork/accumulate/internal/accumulated"
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
	"github.com/tendermint/tendermint/privval"
	tmtypes "github.com/tendermint/tendermint/types"
)

const logConsole = false

func RunTestNet(t *testing.T, subnets []string, daemons map[string][]*accumulated.Daemon, openDb func(d *accumulated.Daemon) (*database.Database, error), doGenesis bool) map[string][]*FakeNode {
	t.Helper()

	allNodes := map[string][]*FakeNode{}
	for _, netName := range subnets {
		daemons := daemons[netName]
		nodes := make([]*FakeNode, len(daemons))
		allNodes[netName] = nodes
		for i, daemon := range daemons {
			nodes[i] = StartFake(t, daemon, openDb, doGenesis)
		}
	}
	return allNodes
}

func StartFake(t *testing.T, d *accumulated.Daemon, openDb func(d *accumulated.Daemon) (*database.Database, error), doGenesis bool) *FakeNode {
	pv, err := privval.LoadFilePV(
		d.Config.PrivValidator.KeyFile(),
		d.Config.PrivValidator.StateFile(),
	)
	require.NoError(t, err)

	n := new(FakeNode)
	n.t = t
	n.key = pv.Key.PrivKey
	n.network = &d.Config.Accumulate.Network

	var logWriter io.Writer
	if logConsole {
		logWriter, err = logging.NewConsoleWriter(d.Config.LogFormat)
	} else {
		logWriter, err = logging.TestLogWriter(t)(d.Config.LogFormat)
	}
	require.NoError(t, err)
	logLevel, logWriter, err := logging.ParseLogLevel(d.Config.LogLevel, logWriter)
	require.NoError(t, err)
	logger, err := logging.NewTendermintLogger(zerolog.New(logWriter), logLevel, false)
	require.NoError(t, err)
	d.Logger = logger

	if openDb == nil {
		openDb = func(d *accumulated.Daemon) (*database.Database, error) {
			return database.Open("", true, d.Logger)
		}
	}

	n.db, err = openDb(d)
	require.NoError(t, err)
	batch := n.db.Begin()
	defer batch.Discard()

	ledger := protocol.NewInternalLedger()
	err = batch.Account(n.network.NodeUrl(protocol.Ledger)).GetStateAs(ledger)
	if err == nil {
		n.height = ledger.Index
	} else {
		require.ErrorIs(t, err, storage.ErrNotFound)
	}

	appChan := make(chan abcitypes.Application)
	defer close(appChan)
	n.client = acctesting.NewFakeTendermint(appChan, n.db, n.network, n.key.PubKey().Address(), n.NextHeight, func(err error) {
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
		Key:     n.key.Bytes(),
		Network: *n.network,
	})
	require.NoError(t, err)

	n.app = abci.NewAccumulator(abci.AccumulatorOptions{
		Chain:   mgr,
		DB:      n.db,
		Logger:  logger,
		Network: *n.network,
		Address: pv.Key.Address,
	})
	appChan <- n.app
	n.app.(*abci.Accumulator).OnFatal(func(err error) {
		t.Helper()
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
			{PubKey: n.key.PubKey()},
		},
	})
	require.NoError(t, err)

	state, err := kv.MarshalJSON()
	require.NoError(t, err)

	n.app.InitChain(abcitypes.RequestInitChain{
		Time:          time.Now(),
		ChainId:       d.Config.Accumulate.Network.ID,
		AppStateBytes: state,
	})

	return n
}

type FakeNode struct {
	t       testing.TB
	db      *database.Database
	network *config.Network
	app     abcitypes.Application
	client  *acctesting.FakeTendermint
	query   *accapi.Query
	key     crypto.PrivKey
	height  int64

	assert  *assert.Assertions
	require *require.Assertions
}

func (n *FakeNode) T() testing.TB {
	return n.t
}

func (n *FakeNode) Assert() *assert.Assertions {
	if n.assert == nil {
		n.assert = assert.New(n.T())
	}
	return n.assert
}

func (n *FakeNode) Require() *require.Assertions {
	if n.require == nil {
		n.require = require.New(n.T())
	}
	return n.require
}

func (n *FakeNode) NextHeight() int64 {
	n.height++
	return n.height
}

func (n *FakeNode) Query(q *query.Query) *api.APIDataResponse {
	payload, err := q.MarshalBinary()
	require.NoError(n.t, err)

	resp := n.app.Query(abcitypes.RequestQuery{Data: payload})
	require.Zero(n.t, resp.Code, "Query failed: %s", resp.Info)

	var msg json.RawMessage = []byte(fmt.Sprintf("{\"entry\":\"%x\"}", resp.Value))
	chain := new(state.ChainHeader)
	require.NoError(n.t, chain.UnmarshalBinary(resp.Value))
	return &api.APIDataResponse{Type: types.String(chain.Type.Name()), Data: &msg}
}

func (n *FakeNode) GetChainStateByUrl(url string) *api.APIDataResponse {
	r, err := n.query.GetChainStateByUrl(url)
	require.NoError(n.t, err)
	return r
}

func (n *FakeNode) GetChainDataByUrl(url string) *api.APIDataResponse {
	r, err := n.query.QueryDataByUrl(url)
	require.NoError(n.t, err)
	return r
}

func (n *FakeNode) GetChainDataByEntryHash(url string, entryHash []byte) *api.APIDataResponse {
	r, err := n.query.GetDataByEntryHash(url, entryHash)
	require.NoError(n.t, err)
	return r
}

func (n *FakeNode) GetChainDataSet(url string, start uint64, limit uint64, expand bool) *api.APIDataResponsePagination {
	r, err := n.query.GetDataSetByUrl(url, start, limit, expand)
	require.NoError(n.t, err)
	return r
}

func (n *FakeNode) GetChainStateByTxId(txid []byte) *api.APIDataResponse {
	r, err := n.query.GetChainStateByTxId(txid)
	require.NoError(n.t, err)
	return r
}

func (n *FakeNode) GetChainStateByChainId(txid []byte) *api.APIDataResponse {
	r, err := n.query.GetChainStateByChainId(txid)
	require.NoError(n.t, err)
	return r
}

func (n *FakeNode) Batch(inBlock func(func(*transactions.Envelope))) [][32]byte {
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
		err := n.client.WaitFor(id, true)
		n.Require().NoError(err)
	}
	return ids
}

func (n *FakeNode) ParseUrl(s string) *url.URL {
	u, err := url.Parse(s)
	require.NoError(n.t, err)
	return u
}

func (n *FakeNode) GetDirectory(adi string) []string {
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

func (n *FakeNode) GetTx(txid []byte) *api2.TransactionQueryResponse {
	q := api2.NewQueryDirect(n.client, api2.QuerierOptions{})
	resp, err := q.QueryTx(txid, 0, api2.QueryOptions{})
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

func (n *FakeNode) GetChainAs(url string, obj encoding.BinaryUnmarshaler) {
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

func (n *FakeNode) GetDataAccount(url string) *protocol.DataAccount {
	acct := protocol.NewDataAccount()
	n.GetChainAs(url, acct)
	return acct
}

func (n *FakeNode) GetTokenAccount(url string) *protocol.TokenAccount {
	acct := protocol.NewTokenAccount()
	n.GetChainAs(url, acct)
	return acct
}

func (n *FakeNode) GetLiteTokenAccount(url string) *protocol.LiteTokenAccount {
	acct := new(protocol.LiteTokenAccount)
	n.GetChainAs(url, acct)
	return acct
}

func (n *FakeNode) GetLiteDataAccount(url string) *protocol.LiteDataAccount {
	acct := new(protocol.LiteDataAccount)
	n.GetChainAs(url, acct)
	return acct
}

func (n *FakeNode) GetADI(url string) *protocol.ADI {
	adi := new(protocol.ADI)
	n.GetChainAs(url, adi)
	return adi
}

func (n *FakeNode) GetKeyBook(url string) *protocol.KeyBook {
	book := new(protocol.KeyBook)
	n.GetChainAs(url, book)
	return book
}

func (n *FakeNode) GetKeyPage(url string) *protocol.KeyPage {
	mss := new(protocol.KeyPage)
	n.GetChainAs(url, mss)
	return mss
}

func (n *FakeNode) GetTokenIssuer(url string) *protocol.TokenIssuer {
	mss := new(protocol.TokenIssuer)
	n.GetChainAs(url, mss)
	return mss
}

type e2eDUT struct {
	*e2e.Suite
	*FakeNode
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
