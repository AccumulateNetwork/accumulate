package api_test

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"net"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/AccumulateNetwork/accumulate/internal/api/v2"
	"github.com/AccumulateNetwork/accumulate/internal/testing/e2e"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	. "github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestEndToEnd(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		t.Skip("This test does not work well on Windows or macOS")
	}

	if os.Getenv("CI") == "true" {
		t.Skip("This test consistently fails in CI")
	}

	baseIP := net.ParseIP("127.1.25.1")
	suite.Run(t, e2e.NewSuite(func(s *e2e.Suite) e2e.DUT {
		daemons := startAccumulate(t, baseIP, 1, 2, 3000)
		return &e2eDUT{s, daemons}
	}))
}

func TestValidate(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		t.Skip("This test does not work well on Windows or macOS")
	}

	daemons := startAccumulate(t, net.ParseIP("127.1.26.1"), 2, 2, 3000)
	japi := daemons[0].Jrpc_TESTONLY()

	t.Run("Not found", func(t *testing.T) {
		b, err := json.Marshal(&api.TxnQuery{Txid: make([]byte, 32), Wait: 2 * time.Second})
		require.NoError(t, err)

		r := japi.GetMethod("query-tx")(context.Background(), b)
		err, _ = r.(error)
		require.Error(t, err)
	})

	var liteKey ed25519.PrivateKey
	var liteUrl *url.URL
	t.Run("Faucet", func(t *testing.T) {
		liteKey = newKey([]byte(t.Name()))
		liteUrl = makeLiteUrl(t, liteKey, ACME)

		xr := new(api.TxResponse)
		callApi(t, japi, "faucet", &AcmeFaucet{Url: liteUrl.String()}, xr)
		require.Zero(t, xr.Code, xr.Message)
		txWait(t, japi, xr.Txid)

		account := NewLiteTokenAccount()
		queryAs(t, japi, "query", &api.UrlQuery{Url: liteUrl.String()}, account)
		assert.Equal(t, int64(10*AcmePrecision), account.Balance.Int64())
	})

	t.Run("Lite Account Credits", func(t *testing.T) {
		executeTx(t, japi, "add-credits", true, execParams{
			Origin: liteUrl.String(),
			Key:    liteKey,
			Payload: &AddCredits{
				Recipient: liteUrl.String(),
				Amount:    100,
			},
		})

		account := NewLiteTokenAccount()
		queryAs(t, japi, "query", &api.UrlQuery{Url: liteUrl.String()}, account)
		assert.Equal(t, int64(100), account.CreditBalance.Int64())
		assert.Equal(t, int64(10*AcmePrecision-AcmePrecision/100), account.Balance.Int64())

		query(t, japi, "query-chain", &api.ChainIdQuery{ChainId: liteUrl.ResourceChain()})
	})

	var adiKey ed25519.PrivateKey
	var adiName = "acc://keytest"
	t.Run("Create ADI", func(t *testing.T) {
		adiKey = newKey([]byte(t.Name()))

		executeTx(t, japi, "create-adi", true, execParams{
			Origin: liteUrl.String(),
			Key:    liteKey,
			Payload: &IdentityCreate{
				Url:         adiName,
				PublicKey:   adiKey[32:],
				KeyBookName: "book",
				KeyPageName: "page",
			},
		})

		adi := new(state.AdiState)
		queryAs(t, japi, "query", &api.UrlQuery{Url: adiName}, adi)
		assert.Equal(t, adiName, string(adi.ChainUrl))

		dir := new(api.DirectoryQueryResult)
		queryAs(t, japi, "query-directory", struct {
			Url          string
			Count        int
			ExpandChains bool
		}{adiName, 10, true}, dir)
		assert.ElementsMatch(t, []string{
			adiName,
			adiName + "/book",
			adiName + "/page",
		}, dir.Entries)
	})

	t.Run("Txn History", func(t *testing.T) {
		r := new(api.QueryMultiResponse)
		callApi(t, japi, "query-tx-history", struct {
			Url   string
			Count int
		}{liteUrl.String(), 10}, r)
		require.Len(t, r.Items, 3)
	})
}
