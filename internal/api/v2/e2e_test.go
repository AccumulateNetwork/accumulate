package api_test

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"testing"
	"time"

	acctesting "github.com/AccumulateNetwork/accumulate/internal/testing"
	"github.com/AccumulateNetwork/accumulate/internal/testing/e2e"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	. "github.com/AccumulateNetwork/accumulate/protocol"
	query2 "github.com/AccumulateNetwork/accumulate/types/api/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestEndToEnd(t *testing.T) {
	acctesting.SkipCI(t, "flaky")
	acctesting.SkipPlatform(t, "windows", "flaky")
	acctesting.SkipPlatform(t, "darwin", "flaky")
	acctesting.SkipPlatformCI(t, "darwin", "requires setting up localhost aliases")

	// Reuse the same IPs for each test
	ips := acctesting.GetIPs(2)

	suite.Run(t, e2e.NewSuite(func(s *e2e.Suite) e2e.DUT {
		daemons := startAccumulate(t, ips, 1, 2, 3000)
		return &e2eDUT{s, daemons}
	}))
}

func TestValidate(t *testing.T) {
	acctesting.SkipPlatform(t, "windows", "flaky")
	acctesting.SkipPlatform(t, "darwin", "flaky")
	acctesting.SkipPlatformCI(t, "darwin", "requires setting up localhost aliases")

	daemons := startAccumulate(t, acctesting.GetIPs(4), 2, 2, 3000)
	japi := daemons[0].Jrpc_TESTONLY()

	t.Log("Not found")
	{
		b, err := json.Marshal(&api.TxnQuery{Txid: make([]byte, 32), Wait: 2 * time.Second})
		require.NoError(t, err)

		r := japi.GetMethod("query-tx")(context.Background(), b)
		err, _ = r.(error)
		require.Error(t, err)
	}

	var liteKey ed25519.PrivateKey
	var liteUrl *url.URL
	t.Log("Faucet")
	{
		liteKey = newKey([]byte(t.Name()))
		liteUrl = makeLiteUrl(t, liteKey, ACME)

		xr := new(api.TxResponse)
		callApi(t, japi, "faucet", &AcmeFaucet{Url: liteUrl.String()}, xr)
		require.Zero(t, xr.Code, xr.Message)
		txWait(t, japi, xr.Txid)

		account := NewLiteTokenAccount()
		queryRecordAs(t, japi, "query", &api.UrlQuery{Url: liteUrl.String()}, account)
		assert.Equal(t, int64(10*AcmePrecision), account.Balance.Int64())
	}

	t.Log("Lite Account Credits")
	{
		executeTx(t, japi, "add-credits", true, execParams{
			Origin: liteUrl.String(),
			Key:    liteKey,
			Payload: &AddCredits{
				Recipient: liteUrl.String(),
				Amount:    100,
			},
		})

		account := NewLiteTokenAccount()
		queryRecordAs(t, japi, "query", &api.UrlQuery{Url: liteUrl.String()}, account)
		assert.Equal(t, int64(100), account.CreditBalance.Int64())
		assert.Equal(t, int64(10*AcmePrecision-AcmePrecision/100), account.Balance.Int64())

		queryRecord(t, japi, "query-chain", &api.ChainIdQuery{ChainId: liteUrl.ResourceChain()})
	}

	var adiKey ed25519.PrivateKey
	var adiName = "acc://keytest"
	t.Log("Create ADI")
	{
		adiKey = newKey([]byte(t.Name()))

		executeTx(t, japi, "create-adi", true, execParams{
			Origin: liteUrl.String(),
			Key:    liteKey,
			Payload: &CreateIdentity{
				Url:         adiName,
				PublicKey:   adiKey[32:],
				KeyBookName: "book",
				KeyPageName: "page",
			},
		})

		adi := new(protocol.ADI)
		queryRecordAs(t, japi, "query", &api.UrlQuery{Url: adiName}, adi)
		assert.Equal(t, adiName, string(adi.ChainUrl))

		dir := new(api.MultiResponse)
		callApi(t, japi, "query-directory", struct {
			Url          string
			Count        int
			ExpandChains bool
		}{adiName, 10, true}, dir)
		assert.ElementsMatch(t, []interface{}{
			adiName,
			adiName + "/book",
			adiName + "/page",
		}, dir.Items)
	}

	t.Log("Txn History")
	{
		r := new(api.MultiResponse)
		callApi(t, japi, "query-tx-history", struct {
			Url   string
			Count int
		}{liteUrl.String(), 10}, r)
		require.Len(t, r.Items, 3)
	}

	dataAccountUrl := adiName + "/dataAccount"
	t.Log("Create Data Account")
	{
		executeTx(t, japi, "create-data-account", true, execParams{
			Origin: adiName,
			Key:    adiKey,
			Payload: &CreateDataAccount{
				Url: dataAccountUrl,
			},
		})
		dataAccount := NewDataAccount()
		queryRecordAs(t, japi, "query", &api.UrlQuery{Url: dataAccountUrl}, dataAccount)
		assert.Equal(t, dataAccountUrl, string(dataAccount.ChainUrl))
	}

	keyPageUrl := adiName + "/page1"
	t.Log("Create Key Page")
	{
		var keys []*KeySpecParams
		keys = append(keys, &KeySpecParams{
			PublicKey: adiKey,
		})
		executeTx(t, japi, "create-key-page", true, execParams{
			Origin: adiName,
			Key:    adiKey,
			Payload: &CreateKeyPage{
				Url:  keyPageUrl,
				Keys: keys,
			},
		})
		keyPage := NewKeyPage()
		queryRecordAs(t, japi, "query", &api.UrlQuery{Url: keyPageUrl}, keyPage)
		assert.Equal(t, keyPageUrl, string(keyPage.ChainUrl))
	}

	keyBookUrl := adiName + "/book1"
	t.Log("Create Key Book")
	{
		var page []string
		pageUrl := makeUrl(t, keyPageUrl)
		page = append(page, pageUrl.String())
		executeTx(t, japi, "create-key-book", true, execParams{
			Origin: adiName,
			Key:    adiKey,
			Payload: &CreateKeyBook{
				Url:   keyBookUrl,
				Pages: page,
			},
		})
		keyBook := NewKeyBook()
		queryRecordAs(t, japi, "query", &api.UrlQuery{Url: keyBookUrl}, keyBook)
		assert.Equal(t, keyBookUrl, string(keyBook.ChainUrl))
	}

	tokenUrl := "acc://ACME"
	tokenAccountUrl := adiName + "/account"
	t.Log("Create Token Account")
	{
		executeTx(t, japi, "create-token-account", true, execParams{
			Origin: adiName,
			Key:    adiKey,
			Payload: &CreateTokenAccount{
				Url:        tokenAccountUrl,
				TokenUrl:   tokenUrl,
				KeyBookUrl: keyBookUrl,
			},
		})
		tokenAccount := NewLiteTokenAccount()
		queryRecordAs(t, japi, "query", &api.UrlQuery{Url: tokenAccountUrl}, tokenAccount)
		assert.Equal(t, tokenAccountUrl, string(tokenAccount.ChainUrl))
	}

	t.Log("Query Key Index")
	{
		keyIndex := &query2.ResponseKeyPageIndex{}
		queryRecordAs(t, japi, "query-key-index", &api.KeyPageIndexQuery{
			UrlQuery: api.UrlQuery{
				Url: keyPageUrl,
			},
			Key: adiKey,
		}, keyIndex)
		assert.Equal(t, keyPageUrl, keyIndex.KeyPage)
	}

}

func TestTokenTransfer(t *testing.T) {
	acctesting.SkipPlatform(t, "windows", "flaky")
	acctesting.SkipPlatform(t, "darwin", "flaky")
	acctesting.SkipPlatformCI(t, "darwin", "requires setting up localhost aliases")

	daemons := startAccumulate(t, acctesting.GetIPs(4), 2, 2, 3000)

	var aliceKey ed25519.PrivateKey
	var aliceUrl *url.URL
	var bobKey ed25519.PrivateKey
	var bobUrl *url.URL
	t.Run("Send Token", func(t *testing.T) {
		bobKey = newKey([]byte(t.Name()))
		bobUrl = makeLiteUrl(t, bobKey, ACME)
		aliceKey = newKey([]byte(t.Name()))
		aliceUrl = makeLiteUrl(t, aliceKey, ACME)

		var to []*protocol.TokenRecipient
		to = append(to, &protocol.TokenRecipient{
			Url:    aliceUrl.String(),
			Amount: uint64(100),
		})
		txParams := execParams{
			Origin: bobUrl.String(),
			Key:    bobKey,
			Payload: &protocol.SendTokens{
				To: to,
			},
		}

		// Ensure we see the not found error code regardless of which
		// node on which BVN the transaction is sent to
		for i, daemon := range daemons {
			japi := daemon.Jrpc_TESTONLY()
			res := executeTxFail(t, japi, "send-tokens", 1, txParams)
			assert.Equal(t, uint64(protocol.CodeNotFound), res.Code, "Node %d (BVN %d) returned the wrong error code", i, i/2)
		}
	})

}
