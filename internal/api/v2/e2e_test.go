package api_test

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	query2 "gitlab.com/accumulatenetwork/accumulate/types/api/query"
)

func init() { acctesting.EnableDebugFeatures() }

func TestStatus(t *testing.T) {
	partitions, daemons := acctesting.CreateTestNet(t, 2, 2, 0, false)
	acctesting.RunTestNet(t, partitions, daemons)
	japi := daemons["BVN1"][0].Jrpc_TESTONLY()

	// Create some history
	liteUrl := makeLiteUrl(t, newKey([]byte(t.Name())), ACME)
	xr := new(api.TxResponse)
	callApi(t, japi, "faucet", &AcmeFaucet{Url: liteUrl}, xr)
	require.Zero(t, xr.Code, xr.Message)
	txWait(t, japi, xr.TransactionHash)

	// Test
	r := japi.Status(context.Background(), nil)
	if err, ok := r.(error); ok {
		require.NoError(t, err)
	}
	require.IsType(t, (*api.StatusResponse)(nil), r)
	status := r.(*api.StatusResponse)

	// Check the status
	assert.True(t, status.Ok, "Ok should be true")
	assert.NotZero(t, status.LastDirectoryAnchorHeight, "Last directory anchor height should be non-zero")
	assert.NotZero(t, status.BvnHeight, "Height should be non-zero")
	assert.NotZero(t, status.BvnRootHash, "Root hash should be present")
	assert.NotZero(t, status.BvnBptHash, "BPT hash should be present")
}

func TestValidate(t *testing.T) {
	acctesting.SkipCI(t, "flaky")
	acctesting.SkipPlatform(t, "windows", "flaky")
	acctesting.SkipPlatform(t, "darwin", "flaky")
	acctesting.SkipPlatformCI(t, "darwin", "requires setting up localhost aliases")
	t.Skip("flaky")
	partitions, daemons := acctesting.CreateTestNet(t, 2, 2, 0, false)
	acctesting.RunTestNet(t, partitions, daemons)
	japi := daemons[Directory][0].Jrpc_TESTONLY()

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
			callApi(t, japi, "faucet", &AcmeFaucet{Url: liteUrl}, xr)
			require.Zero(t, xr.Code, xr.Message)
			txWait(t, japi, xr.TransactionHash)
		}

		account := new(LiteTokenAccount)
		queryRecordAs(t, japi, "query", &api.UrlQuery{Url: liteUrl}, account)
		assert.Equal(t, int64(count*AcmeFaucetAmount*AcmePrecision), account.Balance.Int64())
	})

	t.Run("Lite Token Identity Credits", func(t *testing.T) {
		executeTx(t, japi, "add-credits", true, execParams{
			Origin: liteUrl.String(),
			Key:    liteKey,
			Payload: &AddCredits{
				Recipient: liteUrl,
				Amount:    *big.NewInt(1e10),
			},
		})

		account := new(LiteIdentity)
		queryRecordAs(t, japi, "query", &api.UrlQuery{Url: liteUrl}, account)
		assert.Equal(t, uint64(1e5), account.CreditBalance)

		queryRecord(t, japi, "query-chain", &api.ChainIdQuery{ChainId: liteUrl.AccountID()})
	})

	var adiKey ed25519.PrivateKey
	var adiName = &url.URL{Authority: "keytest"}
	t.Run("Create ADI", func(t *testing.T) {
		adiKey = newKey([]byte(t.Name()))

		bookUrl, err := url.Parse(fmt.Sprintf("%s/book", adiName))
		require.NoError(t, err)

		executeTx(t, japi, "create-adi", true, execParams{
			Origin: liteUrl.String(),
			Key:    liteKey,
			Payload: &CreateIdentity{
				Url:        adiName,
				KeyHash:    adiKey[32:],
				KeyBookUrl: bookUrl,
			},
		})

		adi := new(ADI)
		queryRecordAs(t, japi, "query", &api.UrlQuery{Url: adiName}, adi)
		assert.Equal(t, adiName, adi.Url)

		dir := new(api.MultiResponse)
		callApi(t, japi, "query-directory", struct {
			Url          string
			Count        int
			ExpandChains bool
		}{adiName.String(), 10, true}, dir)
		assert.ElementsMatch(t, []interface{}{
			adiName.String(),
			adiName.JoinPath("/book").String(),
			adiName.JoinPath("/page").String(),
		}, dir.Items)
	})

	t.Run("Key page credits", func(t *testing.T) {
		pageUrl := adiName.JoinPath("/page")
		executeTx(t, japi, "add-credits", true, execParams{
			Origin: liteUrl.String(),
			Key:    liteKey,
			Payload: &AddCredits{
				Recipient: pageUrl,
				Amount:    *big.NewInt(1e5),
			},
		})

		page := new(KeyPage)
		queryRecordAs(t, japi, "query", &api.UrlQuery{Url: pageUrl}, page)
		assert.Equal(t, uint64(1e5), page.CreditBalance)
	})

	t.Run("Txn History", func(t *testing.T) {
		r := new(api.MultiResponse)
		callApi(t, japi, "query-tx-history", struct {
			Url   string
			Count int
		}{liteUrl.String(), 10}, r)
		require.Equal(t, 7, len(r.Items), "Expected 7 transactions for %s", liteUrl)
	})

	dataAccountUrl := adiName.JoinPath("/dataAccount")
	t.Run("Create Data Account", func(t *testing.T) {
		executeTx(t, japi, "create-data-account", true, execParams{
			Origin: adiName.String(),
			Key:    adiKey,
			Payload: &CreateDataAccount{
				Url: dataAccountUrl,
			},
		})
		dataAccount := new(DataAccount)
		queryRecordAs(t, japi, "query", &api.UrlQuery{Url: dataAccountUrl}, dataAccount)
		assert.Equal(t, dataAccountUrl, dataAccount.Url)
	})

	keyBookUrl := adiName.JoinPath("/book1")
	t.Run("Create Key Book", func(t *testing.T) {
		executeTx(t, japi, "create-key-book", true, execParams{
			Origin: adiName.String(),
			Key:    adiKey,
			Payload: &CreateKeyBook{
				Url: keyBookUrl,
			},
		})
		keyBook := new(KeyBook)
		queryRecordAs(t, japi, "query", &api.UrlQuery{Url: keyBookUrl}, keyBook)
		assert.Equal(t, keyBookUrl, keyBook.Url)
	})

	keyPageUrl := FormatKeyPageUrl(keyBookUrl, 0)
	t.Run("Create Key Page", func(t *testing.T) {
		var keys []*KeySpecParams
		// pubKey, _ := json.Marshal(adiKey.Public())
		keys = append(keys, &KeySpecParams{
			KeyHash: adiKey[32:],
		})
		executeTx(t, japi, "create-key-page", true, execParams{
			Origin: keyBookUrl.String(),
			Key:    adiKey,
			Payload: &CreateKeyPage{
				Keys: keys,
			},
		})

		keyPage := new(KeyPage)
		queryRecordAs(t, japi, "query", &api.UrlQuery{Url: keyPageUrl}, keyPage)
		assert.Equal(t, keyPageUrl, keyPage.Url)
	})

	t.Run("Key page credits 2", func(t *testing.T) {
		executeTx(t, japi, "add-credits", true, execParams{
			Origin: liteUrl.String(),
			Key:    liteKey,
			Payload: &AddCredits{
				Recipient: keyPageUrl,
				Amount:    *big.NewInt(1e5),
			},
		})

		page := new(KeyPage)
		queryRecordAs(t, japi, "query", &api.UrlQuery{Url: keyPageUrl}, page)
		assert.Equal(t, uint64(1e5), page.CreditBalance)
	})

	var adiKey2 ed25519.PrivateKey
	t.Run("Update Key Page", func(t *testing.T) {
		adiKey2 = newKey([]byte(t.Name()))

		executeTx(t, japi, "update-key-page", true, execParams{
			Origin: keyPageUrl.String(),
			Key:    adiKey,
			Payload: &UpdateKeyPage{
				Operation: []KeyPageOperation{&AddKeyOperation{
					Entry: KeySpecParams{
						KeyHash:  adiKey2[32:],
						Delegate: makeUrl(t, "acc://foo/book1"),
					},
				}},
			},
		})
		keyPage := new(KeyPage)
		queryRecordAs(t, japi, "query", &api.UrlQuery{Url: keyPageUrl}, keyPage)
		assert.Equal(t, "acc://foo/book1", keyPage.Keys[1].Delegate.String())
	})

	tokenAccountUrl := adiName.JoinPath("/account")
	t.Run("Create Token Account", func(t *testing.T) {
		executeTx(t, japi, "create-token-account", true, execParams{
			Origin: adiName.String(),
			Key:    adiKey,
			Payload: &CreateTokenAccount{
				Url:         tokenAccountUrl,
				TokenUrl:    AcmeUrl(),
				Authorities: []*url.URL{keyBookUrl},
			},
		})
		tokenAccount := new(LiteTokenAccount)
		queryRecordAs(t, japi, "query", &api.UrlQuery{Url: tokenAccountUrl}, tokenAccount)
		assert.Equal(t, tokenAccountUrl, tokenAccount.Url)
	})

	t.Run("Query Key Index", func(t *testing.T) {
		keyIndex := &query2.ResponseKeyPageIndex{}
		queryRecordAs(t, japi, "query-key-index", &api.KeyPageIndexQuery{
			UrlQuery: api.UrlQuery{
				Url: keyPageUrl,
			},
			Key: adiKey[32:],
		}, keyIndex)
		assert.Equal(t, keyPageUrl, keyIndex.Signer)
	})
}
