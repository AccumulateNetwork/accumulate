package abci_test

import (
	"context"
	"crypto/ed25519"
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/abci"
	accapi "github.com/AccumulateNetwork/accumulate/internal/api"
	"github.com/AccumulateNetwork/accumulate/internal/chain"
	"github.com/AccumulateNetwork/accumulate/internal/genesis"
	"github.com/AccumulateNetwork/accumulate/internal/logging"
	"github.com/AccumulateNetwork/accumulate/internal/relay"
	acctesting "github.com/AccumulateNetwork/accumulate/internal/testing"
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
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	tmtypes "github.com/tendermint/tendermint/types"
)

func createAppWithMemDB(t testing.TB, addr crypto.Address, logLevel string, doGenesis bool) *fakeNode {
	db, err := state.OpenDBInMemory(&state.OpenOptions{Debug: true})
	require.NoError(t, err)

	return createApp(t, db, addr, logLevel, doGenesis)
}

func createApp(t testing.TB, db *state.StateDB, addr crypto.Address, logLevel string, doGenesis bool) *fakeNode {
	_, bvcKey, _ := ed25519.GenerateKey(rand)

	n := new(fakeNode)
	n.t = t
	n.db = db

	zl := logging.NewTestZeroLogger(t, "plain")
	zl = zl.Hook(logging.ExcludeMessages("GetIndex", "WriteIndex"))
	zl = zl.Hook(logging.BodyHook(func(e *zerolog.Event, _ zerolog.Level, body map[string]interface{}) {
		module, ok := body["module"].(string)
		if !ok {
			return
		}

		switch module {
		case "db":
		// case "accumulate", "db":
		// OK
		default:
			e.Discard()
		}
	}))

	logger, err := logging.NewTendermintLogger(zl, logLevel, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to parse log level: %v", err)
		os.Exit(1)
	}

	appChan := make(chan abcitypes.Application)
	defer close(appChan)

	n.height, err = db.BlockIndex()
	if !errors.Is(err, storage.ErrNotFound) {
		require.NoError(t, err)
	}

	n.client = acctesting.NewABCIApplicationClient(appChan, db, n.NextHeight, func(err error) {
		t.Helper()
		assert.NoError(t, err)
	}, 100*time.Millisecond)
	relay := relay.New(n.client)
	require.NoError(t, relay.Start())
	t.Cleanup(func() { require.NoError(t, relay.Stop()) })
	n.query = accapi.NewQuery(relay)

	mgr, err := chain.NewBlockValidatorExecutor(chain.ExecutorOptions{
		Query:  n.query,
		Local:  n.client,
		DB:     n.db,
		Logger: logger,
		Key:    bvcKey,
	})
	require.NoError(t, err)

	n.app, err = abci.NewAccumulator(db, addr, mgr, logger)
	require.NoError(t, err)
	appChan <- n.app
	n.app.(*abci.Accumulator).OnFatal(func(err error) { require.NoError(t, err) })

	t.Cleanup(func() { n.client.Shutdown() })

	if !doGenesis {
		return n
	}

	n.height++

	kv := new(memory.DB)
	_ = kv.InitDB("", nil)
	_, err = genesis.Init(kv, genesis.InitOpts{
		SubnetID:    t.Name(),
		NetworkType: config.BlockValidator,
		GenesisTime: time.Now(),
		Validators: []tmtypes.GenesisValidator{
			{PubKey: tmed25519.PrivKey(bvcKey).PubKey()},
		},
	})
	require.NoError(t, err)

	state, err := kv.MarshalJSON()
	require.NoError(t, err)

	n.app.InitChain(abcitypes.RequestInitChain{
		Time:          time.Now(),
		ChainId:       t.Name(),
		AppStateBytes: state,
	})

	return n
}

type fakeNode struct {
	t      testing.TB
	db     *state.StateDB
	app    abcitypes.Application
	client *acctesting.ABCIApplicationClient
	query  *accapi.Query
	height int64
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

func (n *fakeNode) Batch(inBlock func(func(*transactions.GenTransaction))) {
	n.t.Helper()

	inBlock(func(tx *transactions.GenTransaction) {
		b, err := tx.Marshal()
		require.NoError(n.t, err)
		n.client.SubmitTx(context.Background(), b)
	})

	n.client.Wait()
}

func generateKey() tmed25519.PrivKey {
	_, key, _ := ed25519.GenerateKey(rand)
	return tmed25519.PrivKey(key)
}

func edSigner(key tmed25519.PrivKey, nonce uint64) func(hash []byte) (*transactions.ED25519Sig, error) {
	return func(hash []byte) (*transactions.ED25519Sig, error) {
		sig := new(transactions.ED25519Sig)
		return sig, sig.Sign(1, key, hash)
	}
}

func (n *fakeNode) ParseUrl(s string) *url.URL {
	u, err := url.Parse(s)
	require.NoError(n.t, err)
	return u
}

func (n *fakeNode) GetDirectory(adi string) []string {
	u := n.ParseUrl(adi)
	require.True(n.t, u.Identity().Equal(u))

	md := new(protocol.DirectoryIndexMetadata)
	idc := u.IdentityChain()
	b, err := n.db.GetIndex(state.DirectoryIndex, idc, "Metadata")
	if errors.Is(err, storage.ErrNotFound) {
		return nil
	}
	require.NoError(n.t, err)
	require.NoError(n.t, md.UnmarshalBinary(b))

	chains := make([]string, md.Count)
	for i := range chains {
		b, err := n.db.GetIndex(state.DirectoryIndex, idc, uint64(i))
		require.NoError(n.t, err)
		chains[i] = string(b)
	}
	return chains
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

func (n *fakeNode) GetTokenAccount(url string) *state.TokenAccount {
	acct := new(state.TokenAccount)
	n.GetChainAs(url, acct)
	return acct
}

func (n *fakeNode) GetLiteTokenAccount(url string) *protocol.LiteTokenAccount {
	acct := new(protocol.LiteTokenAccount)
	n.GetChainAs(url, acct)
	return acct
}

func (n *fakeNode) GetADI(url string) *state.AdiState {
	adi := new(state.AdiState)
	n.GetChainAs(url, adi)
	return adi
}

func (n *fakeNode) GetKeyBook(url string) *protocol.KeyBook {
	ssg := new(protocol.KeyBook)
	n.GetChainAs(url, ssg)
	return ssg
}

func (n *fakeNode) GetKeyPage(url string) *protocol.KeyPage {
	mss := new(protocol.KeyPage)
	n.GetChainAs(url, mss)
	return mss
}

type e2eDUT struct {
	*fakeNode
}

func (n e2eDUT) GetUrl(url string) (*ctypes.ResultABCIQuery, error) {
	return n.query.QueryByUrl(url)
}

func (n e2eDUT) SubmitTxn(tx *transactions.GenTransaction) {
	b, err := tx.Marshal()
	require.NoError(n.t, err)
	n.client.SubmitTx(context.Background(), b)
}

func (n e2eDUT) WaitForTxns() {
	n.client.Wait()
}
