package abci_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

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
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/blockscheduler"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/connections"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/testing/e2e"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

type FakeNode struct {
	t       testing.TB
	db      *database.Database
	network *config.Describe
	exec    *block.Executor
	app     abcitypes.Application
	client  *acctesting.FakeTendermint
	key     crypto.PrivKey
	height  int64
	api     *client.Client
	logger  log.Logger
	router  routing.Router

	assert  *assert.Assertions
	require *require.Assertions
}

func RunTestNet(t *testing.T, partitions []string, daemons map[string][]*accumulated.Daemon, openDb func(d *accumulated.Daemon) (*database.Database, error), doGenesis bool, errorHandler func(err error)) map[string][]*FakeNode {
	t.Helper()

	allNodes := map[string][]*FakeNode{}
	allChans := map[string][]chan<- abcitypes.Application{}
	evilNodePrefix := "evil-"
	for i, netName := range partitions {
		isEvil := false
		if strings.HasPrefix(netName, evilNodePrefix) {
			isEvil = true
			netName = strings.TrimPrefix(netName, evilNodePrefix)
			partitions[i] = netName
		}

		daemons := daemons[netName]
		nodes := make([]*FakeNode, len(daemons))
		chans := make([]chan<- abcitypes.Application, len(daemons))
		allNodes[netName], allChans[netName] = nodes, chans
		for i, daemon := range daemons {
			nodes[i], chans[i] = InitFake(t, daemon, openDb, errorHandler, isEvil)
		}
	}

	genesis := map[string]*tmtypes.GenesisDoc{}
	if doGenesis {
		for id, daemons := range daemons {
			var err error
			genesis[id], err = tmtypes.GenesisDocFromFile(daemons[0].Config.GenesisFile())
			require.NoError(t, err)
		}
	}

	connectionManager := connections.NewFakeConnectionManager(partitions)
	for _, netName := range partitions {
		netName = strings.TrimPrefix(netName, evilNodePrefix)
		nodes, chans := allNodes[netName], allChans[netName]
		for i := range nodes {
			nodes[i].Start(chans[i], connectionManager, genesis[netName])
		}
	}

	clients := map[string]connections.FakeClient{}
	for _, netName := range partitions {
		nodes := allNodes[netName]
		// TODO It _should_ be one or the other - why doesn't that work?
		clients[netName] = connections.FakeClient{nodes[0].client, nodes[0].api}
	}
	connectionManager.SetClients(clients)

	return allNodes
}

func NewDefaultErrorHandler(t *testing.T) func(err error) {
	return func(err error) {
		t.Helper()
		assert.NoError(t, err)
	}
}

func InitFake(t *testing.T, d *accumulated.Daemon, openDb func(d *accumulated.Daemon) (*database.Database, error), errorHandler func(err error), isEvil bool) (*FakeNode, chan<- abcitypes.Application) {
	if errorHandler == nil {
		errorHandler = NewDefaultErrorHandler(t)
	}

	pv, err := privval.LoadFilePV(
		d.Config.PrivValidator.KeyFile(),
		d.Config.PrivValidator.StateFile(),
	)
	require.NoError(t, err)

	n := new(FakeNode)
	n.t = t
	n.key = pv.Key.PrivKey
	n.network = &d.Config.Accumulate.Describe
	n.logger = d.Logger

	if openDb == nil {
		openDb = func(d *accumulated.Daemon) (*database.Database, error) {
			return database.OpenInMemory(d.Logger), nil
		}
	}

	n.db, err = openDb(d)
	require.NoError(t, err)
	batch := n.db.Begin(false)
	defer batch.Discard()

	var ledger *protocol.SystemLedger
	err = batch.Account(n.network.NodeUrl(protocol.Ledger)).GetStateAs(&ledger)
	if err == nil {
		n.height = int64(ledger.Index)
	} else {
		require.ErrorIs(t, err, storage.ErrNotFound)
	}

	fakeTmLogger := d.Logger.With("module", "fake-tendermint", "partition", n.network.PartitionId)

	appChan := make(chan abcitypes.Application)
	t.Cleanup(func() { close(appChan) })
	n.client = acctesting.NewFakeTendermint(appChan, n.db, n.network, n.key.PubKey(), fakeTmLogger, n.NextHeight, errorHandler, 100*time.Millisecond, isEvil)

	return n, appChan
}

func (n *FakeNode) Start(appChan chan<- abcitypes.Application, connMgr connections.ConnectionManager, genesis *tmtypes.GenesisDoc) *FakeNode {
	eventBus := events.NewBus(nil)
	n.router = routing.NewRouter(eventBus, connMgr)

	var err error
	execOpts := block.ExecutorOptions{
		Logger:   n.logger,
		Key:      n.key.Bytes(),
		Describe: *n.network,
		Router:   n.router,
		EventBus: eventBus,
	}
	// On DNs initialize the major block scheduler
	if execOpts.Describe.NetworkType == config.Directory {
		execOpts.MajorBlockScheduler = blockscheduler.Init(execOpts.EventBus)
	}

	n.exec, err = block.NewNodeExecutor(execOpts, n.db)
	n.Require().NoError(err)

	n.app = abci.NewAccumulator(abci.AccumulatorOptions{
		Executor: n.exec,
		EventBus: eventBus,
		DB:       n.db,
		Logger:   n.logger,
		Config: &config.Config{Accumulate: config.Accumulate{
			Describe: *n.network,
			Snapshots: config.Snapshots{
				Directory: filepath.Join(n.t.TempDir(), "snapshots"),
			},
		}},
		Address: n.key.PubKey().Address(),
	})
	n.app.(*abci.Accumulator).OnFatal(func(err error) {
		n.T().Helper()
		n.Require().NoError(err)
	})

	// Notify FakeTendermint that we have created an ABCI, but don't notify it
	// immediately, but make sure it definitely happens
	defer func() { appChan <- n.app }()

	jrpc, err := api.NewJrpc(api.Options{
		Logger:        n.logger,
		Describe:      n.network,
		Router:        n.router,
		TxMaxWaitTime: 10 * time.Second,
		Database:      n.db,
		Key:           n.key.Bytes(),
	})
	require.NoError(n.t, err)
	n.api = acctesting.DirectJrpcClient(jrpc)

	n.T().Cleanup(func() { n.client.Shutdown() })

	if genesis == nil {
		return n
	}

	n.height++

	n.app.InitChain(abcitypes.RequestInitChain{
		Time:          genesis.GenesisTime,
		ChainId:       n.network.PartitionId,
		AppStateBytes: genesis.AppState,
		InitialHeight: genesis.InitialHeight + 1,
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

func (n *FakeNode) queryUrl(url string, resp interface{}) error {
	req := new(api.GeneralQuery)
	req.Url = n.parseUrl(url)
	return n.api.RequestAPIv2(context.Background(), "query", req, resp)
}

func (n *FakeNode) QueryAccount(url string) *api.ChainQueryResponse {
	n.t.Helper()
	var r *api.ChainQueryResponse
	n.Require().NoError(n.queryUrl(url, &r))
	return r
}

func (n *FakeNode) QueryTransaction(url string) *api.TransactionQueryResponse {
	var r *api.TransactionQueryResponse
	n.require.NoError(n.queryUrl(url, &r))
	return r
}

func (n *FakeNode) QueryMulti(url string) *api.MultiResponse {
	var r *api.MultiResponse
	n.require.NoError(n.queryUrl(url, &r))
	return r
}

func (n *FakeNode) QueryAccountAs(url string, result interface{}) {
	n.t.Helper()
	r := n.QueryAccount(url)
	data, err := json.Marshal(r.Data)
	n.Require().NoError(err)
	n.Require().NoError(json.Unmarshal(data, result))
}

func (n *FakeNode) Execute(inBlock func(func(*protocol.Envelope))) (sigHashes, txnHashes [][32]byte, err error) {
	n.t.Helper()

	bulk := new(protocol.Envelope)
	inBlock(func(tx *protocol.Envelope) {
		deliveries, err := chain.NormalizeEnvelope(tx)
		require.NoError(n.t, err)
		for _, d := range deliveries {
			bulk.Signatures = append(bulk.Signatures, d.Signatures...)
			bulk.Transaction = append(bulk.Transaction, d.Transaction)
		}
	})

	for _, sig := range bulk.Signatures {
		sigHashes = append(sigHashes, *(*[32]byte)(sig.Hash()))
	}
	for _, txn := range bulk.Transaction {
		remote, ok := txn.Body.(*protocol.RemoteTransaction)
		switch {
		case !ok:
			txnHashes = append(txnHashes, *(*[32]byte)(txn.GetHash()))
		case remote.Hash != [32]byte{}:
			txnHashes = append(txnHashes, remote.Hash)
		}
	}

	blob, err := bulk.MarshalBinary()
	require.NoError(n.t, err)

	// Submit all the transactions as a batch
	st := n.client.SubmitTx(context.Background(), blob, true)
	n.require.NotNil(st)
	if st.CheckResult != nil && st.CheckResult.Code != 0 {
		d, err := st.CheckResult.MarshalJSON()
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("%s", d)
	}

	return sigHashes, txnHashes, nil
}

func (n *FakeNode) CheckTx(env *protocol.Envelope) abcitypes.ResponseCheckTx {
	tx, err := env.MarshalBinary()
	n.Require().NoError(err)
	return n.app.CheckTx(abcitypes.RequestCheckTx{Tx: tx})
}

func (n *FakeNode) MustExecute(inBlock func(func(*protocol.Envelope))) (sigHashes, txnHashes [][32]byte) {
	n.t.Helper()

	sigHashes, txnHashes, err := n.Execute(inBlock)
	if err == nil {
		return sigHashes, txnHashes
	}

	var data struct{ Data []byte }
	if json.Unmarshal([]byte(err.Error()), &data) != nil {
		require.NoError(n.t, err)
	}

	results := new(protocol.TransactionResultSet)
	if results.UnmarshalBinary(data.Data) != nil {
		require.NoError(n.t, err)
	}

	for _, result := range results.Results {
		if result.Error != nil {
			assert.NoError(n.t, result.Error)
		}
	}
	n.t.FailNow()
	panic("unreachable")
}

func (n *FakeNode) MustExecuteAndWait(inBlock func(func(*protocol.Envelope))) [][32]byte {
	n.t.Helper()

	_, txnHashes := n.MustExecute(inBlock)
	n.MustWaitForTxns(convertIds32(txnHashes...)...)
	return txnHashes
}

func (n *FakeNode) MustWaitForTxns(ids ...[]byte) {
	n.t.Helper()

	n.Require().NoError(n.WaitForTxns(ids...))
}

func (n *FakeNode) WaitForTxns(ids ...[]byte) error {
	return n.waitForTxns(nil, false, ids...)
}

func (n *FakeNode) QueryTx(txid []byte, wait time.Duration, ignorePending bool) (*api.TransactionQueryResponse, error) {
	req := new(api.TxnQuery)
	req.Txid = txid
	req.Wait = wait
	req.IgnorePending = ignorePending
	return n.api.QueryTx(context.Background(), req)
}

func (n *FakeNode) waitForTxns(cause []byte, ignorePending bool, ids ...[]byte) error {
	n.t.Helper()

	for _, id := range ids {
		if cause == nil {
			n.logger.Debug("Waiting for transaction", "module", "fake-node", "hash", logging.AsHex(id))
		} else {
			n.logger.Debug("Waiting for transaction", "module", "fake-node", "hash", logging.AsHex(id), "cause", logging.AsHex(cause))
		}
		res, err := n.QueryTx(id, 1*time.Second, ignorePending)
		if err != nil {
			return fmt.Errorf("Failed to query TX %X (%v)", id, err)
		}
		ids := make([][]byte, len(res.Produced))
		for i, id := range res.Produced {
			id := id.Hash()
			ids[i] = id[:]
		}
		err = n.waitForTxns(id, true, ids...)
		if err != nil {
			return err
		}
	}
	return nil
}

func convertIds32(ids ...[32]byte) [][]byte {
	ids2 := make([][]byte, len(ids))
	for i, id := range ids {
		id := id // See docs/developer/rangevarref.md
		ids2[i] = id[:]
	}
	return ids2
}

func (n *FakeNode) parseUrl(s string) *url.URL {
	u, err := acctesting.ParseUrl(s)
	require.NoError(n.t, err)
	return u
}

func (n *FakeNode) GetDirectory(adi string) []string {
	batch := n.db.Begin(false)
	defer batch.Discard()

	u := n.parseUrl(adi)
	dir := indexing.Directory(batch, u)
	count, err := dir.Count()
	if errors.Is(err, storage.ErrNotFound) {
		return nil
	}
	require.NoError(n.t, err)

	chains := make([]string, count)
	for i := range chains {
		data, err := dir.Get(uint64(i))
		require.NoError(n.t, err)
		chains[i] = data.String()
	}
	return chains
}

func (n *FakeNode) GetTx(txid []byte) *api.TransactionQueryResponse {
	resp, err := n.QueryTx(txid, 0, false)
	require.NoError(n.t, err)
	data, err := json.Marshal(resp.Data)
	require.NoError(n.t, err)

	var typ protocol.TransactionType
	require.NoError(n.t, typ.UnmarshalJSON([]byte(strconv.Quote(resp.Type))))

	resp.Data, err = protocol.NewTransactionBody(typ)
	require.NoError(n.t, err)
	require.NoError(n.t, json.Unmarshal(data, resp.Data))

	return resp
}

func (n *FakeNode) GetDataAccount(url string) *protocol.DataAccount {
	n.t.Helper()
	acct := new(protocol.DataAccount)
	n.QueryAccountAs(url, acct)
	return acct
}

func (n *FakeNode) GetTokenAccount(url string) *protocol.TokenAccount {
	n.t.Helper()
	acct := new(protocol.TokenAccount)
	n.QueryAccountAs(url, acct)
	return acct
}

func (n *FakeNode) GetLiteIdentity(url string) *protocol.LiteIdentity {
	n.t.Helper()
	acct := new(protocol.LiteIdentity)
	n.QueryAccountAs(url, acct)
	return acct
}

func (n *FakeNode) GetLiteTokenAccount(url string) *protocol.LiteTokenAccount {
	n.t.Helper()
	acct := new(protocol.LiteTokenAccount)
	n.QueryAccountAs(url, acct)
	return acct
}

func (n *FakeNode) GetLiteDataAccount(url string) *protocol.LiteDataAccount {
	n.t.Helper()
	acct := new(protocol.LiteDataAccount)
	n.QueryAccountAs(url, acct)
	return acct
}

func (n *FakeNode) GetADI(url string) *protocol.ADI {
	n.t.Helper()
	adi := new(protocol.ADI)
	n.QueryAccountAs(url, adi)
	return adi
}

func (n *FakeNode) GetKeyBook(url string) *protocol.KeyBook {
	n.t.Helper()
	book := new(protocol.KeyBook)
	n.QueryAccountAs(url, book)
	return book
}

func (n *FakeNode) GetKeyPage(url string) *protocol.KeyPage {
	n.t.Helper()
	mss := new(protocol.KeyPage)
	n.QueryAccountAs(url, mss)
	return mss
}

func (n *FakeNode) GetOraclePrice() uint64 {
	return n.exec.ActiveGlobals_TESTONLY().Oracle.Price
}

func (n *FakeNode) GetTokenIssuer(url string) *protocol.TokenIssuer {
	n.t.Helper()
	mss := new(protocol.TokenIssuer)
	n.QueryAccountAs(url, mss)
	return mss
}

type e2eDUT struct {
	*e2e.Suite
	*FakeNode
}

func (d *e2eDUT) GetRecordAs(url string, target protocol.Account) {
	d.QueryAccountAs(url, target)
}

func (d *e2eDUT) GetRecordHeight(url string) uint64 {
	return d.QueryAccount(url).MainChain.Height
}

func (d *e2eDUT) SubmitTxn(tx *protocol.Envelope) {
	b, err := tx.MarshalBinary()
	d.Require().NoError(err)
	d.client.SubmitTx(context.Background(), b, false)
}

func (d *e2eDUT) WaitForTxns(ids ...[]byte) {
	d.MustWaitForTxns(ids...)
}
