package abci_test

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/AccumulateNetwork/accumulated/internal/abci"
	accapi "github.com/AccumulateNetwork/accumulated/internal/api"
	"github.com/AccumulateNetwork/accumulated/internal/chain"
	"github.com/AccumulateNetwork/accumulated/internal/relay"
	acctesting "github.com/AccumulateNetwork/accumulated/internal/testing"
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/stretchr/testify/require"
	abcitypes "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/crypto"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
)

func createAppWithMemDB(t testing.TB, addr crypto.Address) *fakeNode {
	appId := sha256.Sum256([]byte("foo bar"))
	db := new(state.StateDB)
	err := db.Open("valacc.db", appId[:], true, true)
	require.NoError(t, err)

	return createApp(t, db, addr)
}

func createApp(t testing.TB, db *state.StateDB, addr crypto.Address) *fakeNode {
	_, bvcKey, _ := ed25519.GenerateKey(rand)

	n := new(fakeNode)
	n.t = t
	n.db = db

	appChan := make(chan abcitypes.Application)
	defer close(appChan)

	n.client = acctesting.NewABCIApplicationClient(appChan, n.NextHeight, func(err error) {
		t.Helper()
		require.NoError(t, err)
	})
	n.query = accapi.NewQuery(relay.New(n.client))

	mgr, err := chain.NewBlockValidator(n.query, db, bvcKey)
	require.NoError(t, err)

	n.app, err = abci.NewAccumulator(db, addr, mgr)
	require.NoError(t, err)
	appChan <- n.app

	return n
}

func mustJSON(t testing.TB, v interface{}) string {
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
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

func (n *fakeNode) Query(q *api.Query) *api.APIDataResponse {
	payload, err := q.MarshalBinary()
	require.NoError(n.t, err)

	resp := n.app.Query(abcitypes.RequestQuery{Data: payload})
	require.Zero(n.t, resp.Code, "Query failed: %s", resp.Info)

	var msg json.RawMessage = []byte(fmt.Sprintf("{\"entry\":\"%x\"}", resp.Value))
	chain := new(state.ChainHeader)
	require.NoError(n.t, chain.UnmarshalBinary(resp.Value))
	return &api.APIDataResponse{Type: types.String(chain.Type.Name()), Data: &msg}
}

func (n *fakeNode) GetChainState(url string, txid []byte) *api.APIDataResponse {
	r, err := n.query.GetChainState(&url, txid)
	require.NoError(n.t, err)
	return r
}

func (n *fakeNode) Batch(inBlock func(func(*transactions.GenTransaction))) {
	n.t.Helper()
	n.client.Batch(inBlock)
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

func (n *fakeNode) GetChainAs(url string, obj encoding.BinaryUnmarshaler) {
	r, err := n.query.Query(url, nil)
	require.NoError(n.t, err)

	if r.Response.Code != 0 {
		n.t.Fatalf("query failed with code %d: %s", r.Response.Code, r.Response.Info)
	}

	require.NoError(n.t, obj.UnmarshalBinary(r.Response.Value))
}

func (n *fakeNode) GetTokenAccount(url string) *state.TokenAccount {
	acct := new(state.TokenAccount)
	n.GetChainAs(url, acct)
	return acct
}

func (n *fakeNode) GetADI(url string) *state.AdiState {
	adi := new(state.AdiState)
	n.GetChainAs(url, adi)
	return adi
}

func (n *fakeNode) GetKeyGroup(url string) *protocol.SigSpecGroup {
	ssg := new(protocol.SigSpecGroup)
	n.GetChainAs(url, ssg)
	return ssg
}

func (n *fakeNode) GetKeySet(url string) *protocol.MultiSigSpec {
	mss := new(protocol.MultiSigSpec)
	n.GetChainAs(url, mss)
	return mss
}
