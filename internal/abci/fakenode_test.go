package abci_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	api2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/block"
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

func RunTestNet(t *testing.T, subnets []string, daemons map[string][]*accumulated.Daemon, openDb func(d *accumulated.Daemon) (*database.Database, error), doGenesis bool, errorHandler func(err error)) map[string][]*FakeNode {
	t.Helper()

	allNodes := map[string][]*FakeNode{}
	allChans := map[string][]chan<- abcitypes.Application{}
	clients := map[string]connections.Client{}
	evilNodePrefix := "evil-"
	for _, netName := range subnets {
		isEvil := false
		if strings.HasPrefix(netName, evilNodePrefix) {
			isEvil = true
			netName = strings.TrimPrefix(netName, evilNodePrefix)
		}

		daemons := daemons[netName]
		nodes := make([]*FakeNode, len(daemons))
		chans := make([]chan<- abcitypes.Application, len(daemons))
		allNodes[netName], allChans[netName] = nodes, chans
		for i, daemon := range daemons {
			nodes[i], chans[i] = InitFake(t, daemon, openDb, errorHandler, isEvil)
		}
		// TODO It _should_ be one or the other - why doesn't that work?
		clients[netName] = nodes[0].client
	}
	connectionManager := connections.NewFakeConnectionManager(clients)
	for _, netName := range subnets {
		netName = strings.TrimPrefix(netName, evilNodePrefix)
		nodes, chans := allNodes[netName], allChans[netName]
		for i := range nodes {
			nodes[i].Start(chans[i], connectionManager, doGenesis)
		}
	}
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
	n.network = &d.Config.Accumulate.Network
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

	var ledger *protocol.InternalLedger
	err = batch.Account(n.network.NodeUrl(protocol.Ledger)).GetStateAs(&ledger)
	if err == nil {
		n.height = ledger.Index
	} else {
		require.ErrorIs(t, err, storage.ErrNotFound)
	}

	fakeTmLogger := d.Logger.With("module", "fake-tendermint", "subnet", n.network.LocalSubnetID)

	appChan := make(chan abcitypes.Application)
	t.Cleanup(func() { close(appChan) })
	n.client = acctesting.NewFakeTendermint(appChan, n.db, n.network, n.key.PubKey(), fakeTmLogger, n.NextHeight, errorHandler, 100*time.Millisecond, isEvil)

	return n, appChan
}

func (n *FakeNode) Start(appChan chan<- abcitypes.Application, connMgr connections.ConnectionManager, doGenesis bool) *FakeNode {
	n.router = &routing.RouterInstance{
		Network:           n.network,
		ConnectionManager: connMgr,
	}
	mgr, err := block.NewNodeExecutor(block.ExecutorOptions{
		Logger:  n.logger,
		Key:     n.key.Bytes(),
		Network: *n.network,
		Router:  n.router,
	}, n.db)
	n.Require().NoError(err)

	n.app = abci.NewAccumulator(abci.AccumulatorOptions{
		Executor: mgr,
		DB:       n.db,
		Logger:   n.logger,
		Network:  *n.network,
		Address:  n.key.PubKey().Address(),
	})
	n.app.(*abci.Accumulator).OnFatal(func(err error) {
		n.T().Helper()
		n.Require().NoError(err)
	})

	// Notify FakeTendermint that we have created an ABCI, but don't notify it
	// immediately, but make sure it definitely happens
	defer func() { appChan <- n.app }()

	n.api = api2.NewQueryDispatch(api2.Options{
		Logger:        n.logger,
		Network:       n.network,
		Router:        n.router,
		TxMaxWaitTime: 10 * time.Second,
	})

	n.T().Cleanup(func() { n.client.Shutdown() })

	if !doGenesis {
		return n
	}

	n.height++

	kv := memory.New(nil)
	_, err = genesis.Init(kv, genesis.InitOpts{
		Network:     *n.network,
		GenesisTime: time.Now(),
		Logger:      n.logger,
		Router:      n.router,
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
		InitialHeight: protocol.GenesisBlock + 1,
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
	n.t.Helper()
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
		assert.Zero(n.t, result.Code, result.Message)
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

func (n *FakeNode) waitForTxns(cause []byte, ignorePending bool, ids ...[]byte) error {
	n.t.Helper()

	for _, id := range ids {
		if cause == nil {
			n.logger.Debug("Waiting for transaction", "module", "fake-node", "hash", logging.AsHex(id))
		} else {
			n.logger.Debug("Waiting for transaction", "module", "fake-node", "hash", logging.AsHex(id), "cause", logging.AsHex(cause))
		}
		res, err := n.api.QueryTx(id, 1*time.Second, ignorePending, api2.QueryOptions{})
		if err != nil {
			return fmt.Errorf("Failed to query TX %X (%v)", id, err)
		}
		err = n.waitForTxns(id, true, convertIds32(res.SyntheticTxids...)...)
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
	require.True(n.t, u.RootIdentity().Equal(u))

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
	resp, err := q.QueryTx(txid, 0, false, api2.QueryOptions{})
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
	n.t.Helper()
	batch := n.db.Begin(true)
	defer batch.Discard()
	ledger := batch.Account(n.network.NodeUrl(protocol.Ledger))
	var ledgerState *protocol.InternalLedger
	require.NoError(n.t, ledger.GetStateAs(&ledgerState))
	return ledgerState.ActiveOracle
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
