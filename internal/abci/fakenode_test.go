package abci_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	abcitypes "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/crypto"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/privval"
	tmtypes "github.com/tendermint/tendermint/types"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/abci"
	"gitlab.com/accumulatenetwork/accumulate/internal/accumulated"
	api2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/connections"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/genesis"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/testing/e2e"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/memory"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
	"gitlab.com/accumulatenetwork/accumulate/types/state"
)

type FakeNode struct {
	t       testing.TB
	db      *database.Database
	network *config.Network
	app     abcitypes.Application
	client  *acctesting.FakeTendermint
	key     crypto.PrivKey
	height  int64
	api     api2.Querier
	logger  log.Logger
	router  routing.Router

	assert  *assert.Assertions
	require *require.Assertions
}

func RunTestNet(t *testing.T, subnets []string, daemons map[string][]*accumulated.Daemon, openDb func(d *accumulated.Daemon) (*database.Database, error), doGenesis bool) map[string][]*FakeNode {
	t.Helper()

	allNodes := map[string][]*FakeNode{}
	allChans := map[string][]chan<- abcitypes.Application{}
	clients := map[string]connections.Client{}
	for _, netName := range subnets {
		daemons := daemons[netName]
		nodes := make([]*FakeNode, len(daemons))
		chans := make([]chan<- abcitypes.Application, len(daemons))
		allNodes[netName], allChans[netName] = nodes, chans
		for i, daemon := range daemons {
			nodes[i], chans[i] = InitFake(t, daemon, openDb)
		}
		// TODO It _should_ be one or the other - why doesn't that work?
		clients[netName] = nodes[0].client
	}
	connectionManager := connections.NewFakeConnectionManager(clients)
	for _, netName := range subnets {
		nodes, chans := allNodes[netName], allChans[netName]
		for i := range nodes {
			nodes[i].Start(chans[i], connectionManager, doGenesis)
		}
	}
	return allNodes
}

func InitFake(t *testing.T, d *accumulated.Daemon, openDb func(d *accumulated.Daemon) (*database.Database, error)) (*FakeNode, chan<- abcitypes.Application) {
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
	if acctesting.LogConsole {
		logWriter, err = logging.NewConsoleWriter(d.Config.LogFormat)
	} else {
		logWriter, err = logging.TestLogWriter(t)(d.Config.LogFormat)
	}
	require.NoError(t, err)
	logLevel, logWriter, err := logging.ParseLogLevel(d.Config.LogLevel+";fake-tendermint=debug", logWriter)
	require.NoError(t, err)
	logger, err := logging.NewTendermintLogger(zerolog.New(logWriter), logLevel, false)
	require.NoError(t, err)
	d.Logger = logger
	n.logger = logger

	if openDb == nil {
		openDb = func(d *accumulated.Daemon) (*database.Database, error) {
			return database.Open("", true, d.Logger)
		}
	}

	n.db, err = openDb(d)
	require.NoError(t, err)
	batch := n.db.Begin(false)
	defer batch.Discard()

	ledger := protocol.NewInternalLedger()
	err = batch.Account(n.network.NodeUrl(protocol.Ledger)).GetStateAs(ledger)
	if err == nil {
		n.height = ledger.Index
	} else {
		require.ErrorIs(t, err, storage.ErrNotFound)
	}

	fakeTmLogger := logger.With("module", "fake-tendermint", "subnet", n.network.LocalSubnetID)

	appChan := make(chan abcitypes.Application)
	t.Cleanup(func() { close(appChan) })
	n.client = acctesting.NewFakeTendermint(appChan, n.db, n.network, n.key.PubKey().Address(), fakeTmLogger, n.NextHeight, func(err error) {
		t.Helper()
		assert.NoError(t, err)
	}, 100*time.Millisecond)

	return n, appChan
}

func (n *FakeNode) Start(appChan chan<- abcitypes.Application, connMgr connections.ConnectionManager, doGenesis bool) *FakeNode {
	n.router = &routing.RouterInstance{
		Network:           n.network,
		ConnectionManager: connMgr,
	}
	mgr, err := chain.NewNodeExecutor(chain.ExecutorOptions{
		DB:      n.db,
		Logger:  n.logger,
		Key:     n.key.Bytes(),
		Network: *n.network,
		Router:  n.router,
	})
	n.Require().NoError(err)

	n.app = abci.NewAccumulator(abci.AccumulatorOptions{
		Chain:   mgr,
		DB:      n.db,
		Logger:  n.logger,
		Network: *n.network,
		Address: n.key.PubKey().Address(),
	})
	appChan <- n.app
	n.app.(*abci.Accumulator).OnFatal(func(err error) {
		n.T().Helper()
		n.Require().NoError(err)
	})

	n.api = api2.NewQueryDispatch(api2.Options{
		Logger:        n.logger,
		Network:       n.network,
		Router:        n.router,
		TxMaxWaitTime: 10 * time.Second,
	})

	n.Require().NoError(mgr.Start())
	n.T().Cleanup(func() { _ = mgr.Stop() })
	n.T().Cleanup(func() { n.client.Shutdown() })

	if !doGenesis {
		return n
	}

	n.height++

	kv := new(memory.DB)
	_ = kv.InitDB("", nil)
	_, err = genesis.Init(kv, genesis.InitOpts{
		Network:     *n.network,
		GenesisTime: time.Now(),
		Logger:      n.logger,
		Validators: []tmtypes.GenesisValidator{
			{PubKey: n.key.PubKey()},
		},
	})
	n.Require().NoError(err)

	state, err := kv.MarshalJSON()
	n.Require().NoError(err)

	n.app.InitChain(abcitypes.RequestInitChain{
		Time:          time.Now(),
		ChainId:       n.network.LocalSubnetID,
		AppStateBytes: state,
	})

	return n
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

func (n *FakeNode) QueryAccount(url string) *api2.ChainQueryResponse {
	r, err := n.api.QueryUrl(n.ParseUrl(url), api2.QueryOptions{})
	n.Require().NoError(err)
	n.Require().IsType((*api2.ChainQueryResponse)(nil), r)
	return r.(*api2.ChainQueryResponse)
}

func (n *FakeNode) QueryTransaction(url string) *api2.TransactionQueryResponse {
	r, err := n.api.QueryUrl(n.ParseUrl(url), api2.QueryOptions{})
	n.require.NoError(err)
	n.Require().IsType((*api2.TransactionQueryResponse)(nil), r)
	return r.(*api2.TransactionQueryResponse)
}

func (n *FakeNode) QueryMulti(url string) *api2.MultiResponse {
	r, err := n.api.QueryUrl(n.ParseUrl(url), api2.QueryOptions{})
	n.require.NoError(err)
	n.Require().IsType((*api2.MultiResponse)(nil), r)
	return r.(*api2.MultiResponse)
}

func (n *FakeNode) QueryAccountAs(url string, result interface{}) {
	r := n.QueryAccount(url)
	data, err := json.Marshal(r.Data)
	n.Require().NoError(err)
	n.Require().NoError(json.Unmarshal(data, result))
}

func (n *FakeNode) Batch(inBlock func(func(*transactions.Envelope))) [][32]byte {
	n.t.Helper()

	var ids [][32]byte
	var blob []byte
	inBlock(func(tx *transactions.Envelope) {
		var id [32]byte
		copy(id[:], tx.GetTxHash())
		ids = append(ids, id)
		b, err := tx.MarshalBinary()
		require.NoError(n.t, err)
		blob = append(blob, b...)
	})

	// Submit all the transactions as a batch
	n.client.SubmitTx(context.Background(), blob)

	n.WaitForTxns32(ids...)
	return ids
}

func (n *FakeNode) WaitForTxns32(ids ...[32]byte) {
	ids2 := make([][]byte, len(ids))
	for i, id := range ids {
		// Make a copy to avoid capturing the loop variable
		id := id
		ids2[i] = id[:]
	}
	n.WaitForTxns(ids2...)
}

func (n *FakeNode) WaitForTxns(ids ...[]byte) {
	for _, id := range ids {
		res, err := n.api.QueryTx(id, 1*time.Second, api2.QueryOptions{})
		n.Require().NoError(err)
		n.WaitForTxns32(res.SyntheticTxids...)
	}
}

func (n *FakeNode) ParseUrl(s string) *url.URL {
	u, err := url.Parse(s)
	require.NoError(n.t, err)
	return u
}

func (n *FakeNode) GetDirectory(adi string) []string {
	batch := n.db.Begin(false)
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
	q := api2.NewQueryDirect(n.network.LocalSubnetID, api2.Options{
		Logger:  n.logger,
		Network: n.network,
		Router:  n.router,
	})
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

func (n *FakeNode) GetDataAccount(url string) *protocol.DataAccount {
	acct := protocol.NewDataAccount()
	n.QueryAccountAs(url, acct)
	return acct
}

func (n *FakeNode) GetTokenAccount(url string) *protocol.TokenAccount {
	acct := protocol.NewTokenAccount()
	n.QueryAccountAs(url, acct)
	return acct
}

func (n *FakeNode) GetLiteTokenAccount(url string) *protocol.LiteTokenAccount {
	acct := new(protocol.LiteTokenAccount)
	n.QueryAccountAs(url, acct)
	return acct
}

func (n *FakeNode) GetLiteDataAccount(url string) *protocol.LiteDataAccount {
	acct := new(protocol.LiteDataAccount)
	n.QueryAccountAs(url, acct)
	return acct
}

func (n *FakeNode) GetADI(url string) *protocol.ADI {
	adi := new(protocol.ADI)
	n.QueryAccountAs(url, adi)
	return adi
}

func (n *FakeNode) GetKeyBook(url string) *protocol.KeyBook {
	book := new(protocol.KeyBook)
	n.QueryAccountAs(url, book)
	return book
}

func (n *FakeNode) GetKeyPage(url string) *protocol.KeyPage {
	mss := new(protocol.KeyPage)
	n.QueryAccountAs(url, mss)
	return mss
}

func (n *FakeNode) GetTokenIssuer(url string) *protocol.TokenIssuer {
	mss := new(protocol.TokenIssuer)
	n.QueryAccountAs(url, mss)
	return mss
}

type e2eDUT struct {
	*e2e.Suite
	*FakeNode
}

func (d *e2eDUT) GetRecordAs(url string, target state.Chain) {
	d.QueryAccountAs(url, target)
}

func (d *e2eDUT) GetRecordHeight(url string) uint64 {
	return d.QueryAccount(url).MainChain.Height
}

func (d *e2eDUT) SubmitTxn(tx *transactions.Envelope) {
	b, err := tx.MarshalBinary()
	d.Require().NoError(err)
	d.client.SubmitTx(context.Background(), b)
}
