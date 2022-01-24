package api_test

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/AccumulateNetwork/accumulate/internal/api/v2"
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

	suite.Run(t, e2e.NewSuite(func(s *e2e.Suite) e2e.DUT {
		subnets, daemons := acctesting.CreateTestNet(s.T(), 1, 2, 0)
		acctesting.RunTestNet(s.T(), subnets, daemons)
		return &e2eDUT{s, daemons[protocol.Directory][0]}
	}))
}

func TestValidate(t *testing.T) {
	acctesting.SkipPlatform(t, "windows", "flaky")
	acctesting.SkipPlatform(t, "darwin", "flaky")
	acctesting.SkipPlatformCI(t, "darwin", "requires setting up localhost aliases")

	subnets, daemons := acctesting.CreateTestNet(t, 2, 2, 0)
	acctesting.RunTestNet(t, subnets, daemons)
	japi := daemons[protocol.Directory][0].Jrpc_TESTONLY()

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

		const count = 3
		for i := 0; i < count; i++ {
			xr := new(api.TxResponse)
			callApi(t, japi, "faucet", &AcmeFaucet{Url: liteUrl.String()}, xr)
			require.Zero(t, xr.Code, xr.Message)
			txWait(t, japi, xr.TransactionHash)
		}

		account := NewLiteTokenAccount()
		queryRecordAs(t, japi, "query", &api.UrlQuery{Url: liteUrl.String()}, account)
		assert.Equal(t, int64(count*10*AcmePrecision), account.Balance.Int64())
	})

	t.Run("Lite Account Credits", func(t *testing.T) {
		executeTx(t, japi, "add-credits", true, execParams{
			Origin: liteUrl.String(),
			Key:    liteKey,
			Payload: &AddCredits{
				Recipient: liteUrl.String(),
				Amount:    1e5,
			},
		})

		account := NewLiteTokenAccount()
		queryRecordAs(t, japi, "query", &api.UrlQuery{Url: liteUrl.String()}, account)
		assert.Equal(t, int64(1e5), account.CreditBalance.Int64())

		queryRecord(t, japi, "query-chain", &api.ChainIdQuery{ChainId: liteUrl.AccountID()})
	})

	var adiKey ed25519.PrivateKey
	var adiName = "acc://keytest"
	t.Run("Create ADI", func(t *testing.T) {
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
	})

	t.Run("Key page credits", func(t *testing.T) {
		pageUrl := adiName + "/page"
		executeTx(t, japi, "add-credits", true, execParams{
			Origin: liteUrl.String(),
			Key:    liteKey,
			Payload: &AddCredits{
				Recipient: pageUrl,
				Amount:    1e5,
			},
		})

		page := NewKeyPage()
		queryRecordAs(t, japi, "query", &api.UrlQuery{Url: pageUrl}, page)
		assert.Equal(t, int64(1e5), page.CreditBalance.Int64())
	})

	t.Run("Txn History", func(t *testing.T) {
		r := new(api.MultiResponse)
		callApi(t, japi, "query-tx-history", struct {
			Url   string
			Count int
		}{liteUrl.String(), 10}, r)
		require.Len(t, r.Items, 6)
	})

	dataAccountUrl := adiName + "/dataAccount"
	t.Run("Create Data Account", func(t *testing.T) {
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
	})

	keyPageUrl := adiName + "/page1"
	t.Run("Create Key Page", func(t *testing.T) {
		var keys []*KeySpecParams
		// pubKey, _ := json.Marshal(adiKey.Public())
		keys = append(keys, &KeySpecParams{
			PublicKey: adiKey[32:],
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
	})

	keyBookUrl := adiName + "/book1"
	t.Run("Create Key Book", func(t *testing.T) {
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
	})

	t.Run("Key page credits 2", func(t *testing.T) {
		executeTx(t, japi, "add-credits", true, execParams{
			Origin: liteUrl.String(),
			Key:    liteKey,
			Payload: &AddCredits{
				Recipient: keyPageUrl,
				Amount:    1e5,
			},
		})

		page := NewKeyPage()
		queryRecordAs(t, japi, "query", &api.UrlQuery{Url: keyPageUrl}, page)
		assert.Equal(t, int64(1e5), page.CreditBalance.Int64())
	})

	var adiKey2 ed25519.PrivateKey
	t.Run("Update Key Page", func(t *testing.T) {
		adiKey2 = newKey([]byte(t.Name()))

		executeTx(t, japi, "update-key-page", true, execParams{
			Origin: keyPageUrl,
			Key:    adiKey,
			Payload: &UpdateKeyPage{
				Operation: protocol.KeyPageOperationAdd,
				NewKey:    adiKey2[32:],
				Owner:     "acc://foo/book1",
			},
		})
		keyPage := NewKeyPage()
		queryRecordAs(t, japi, "query", &api.UrlQuery{Url: keyPageUrl}, keyPage)
		assert.Equal(t, "acc://foo/book1", keyPage.Keys[1].Owner)
	})

	tokenUrl := "acc://ACME"
	tokenAccountUrl := adiName + "/account"
	t.Run("Create Token Account", func(t *testing.T) {
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
	})

	t.Run("Query Key Index", func(t *testing.T) {
		keyIndex := &query2.ResponseKeyPageIndex{}
		queryRecordAs(t, japi, "query-key-index", &api.KeyPageIndexQuery{
			UrlQuery: api.UrlQuery{
				Url: keyPageUrl,
			},
			Key: adiKey[32:],
		}, keyIndex)
		assert.Equal(t, keyPageUrl, keyIndex.KeyPage)
	})
}

func TestTokenTransfer(t *testing.T) {
	// acctesting.SkipPlatform(t, "windows", "flaky")
	acctesting.SkipPlatform(t, "darwin", "flaky")
	acctesting.SkipPlatformCI(t, "darwin", "requires setting up localhost aliases")

	subnets, daemons := acctesting.CreateTestNet(t, 2, 2, 0)
	acctesting.RunTestNet(t, subnets, daemons)

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
			Amount: *big.NewInt(100),
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
		for netName, daemons := range daemons {
			if netName == protocol.Directory {
				continue
			}
			for i, daemon := range daemons {
				japi := daemon.Jrpc_TESTONLY()
				res := executeTxFail(t, japi, "send-tokens", 0, 1, txParams)
				assert.Equal(t, uint64(protocol.CodeNotFound), res.Code, "Node %d (%s) returned the wrong error code", i, netName)
			}
		}
	})

}
