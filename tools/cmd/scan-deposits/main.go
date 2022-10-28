package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"sort"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func main() {
	_ = cmd.Execute()
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

func check(err error) {
	if err != nil {
		fatalf("%v", err)
	}
}

func checkf(err error, format string, otherArgs ...interface{}) {
	if err != nil {
		fatalf(format+": %v", append(otherArgs, err)...)
	}
}

var cmd = &cobra.Command{
	Use:   "scan-deposits <network> <account> <since (format: 02 Jan 06 15:04 -0700)>",
	Short: "Scan an account for deposits since the given major block date",
	Run:   run,
}

var flag = struct {
	Debug   bool
	Backoff uint64
}{}

func init() {
	cmd.Flags().BoolVarP(&flag.Debug, "debug", "d", false, "Debug API requests")
	cmd.Flags().Uint64Var(&flag.Backoff, "backoff", 0, "Sets how far back past the expected block to query for the target major block")
}

func run(_ *cobra.Command, args []string) {
	client, err := client.New(args[0])
	checkf(err, "network")
	account, err := url.Parse(args[1])
	checkf(err, "account")
	since, err := time.Parse(time.RFC822Z, args[2])
	checkf(err, "since: want RFC822 date with numeric zone")

	client.DebugRequest = flag.Debug

	// Route the account
	partition := route(client, account)

	// Calculate how many major blocks there have been
	firstBlock, lastBlock := getMajorBlockRange(client, partition, since)

	// Find the transaction range
	firstTxn, lastTxn := findTransactionRange(client, account, firstBlock, lastBlock)

	req := new(api.TxHistoryQuery)
	req.Url = account
	req.Start = uint64(firstTxn)
	req.Count = uint64(lastTxn - firstTxn)
	res, err := client.QueryTxHistory(context.Background(), req)
	check(err)
	total := new(big.Int)
	for _, raw := range res.Items {
		res := as[api.TransactionQueryResponse](raw)
		switch body := res.Transaction.Body.(type) {
		case *protocol.SyntheticDepositTokens:
			if !body.Token.Equal(protocol.AcmeUrl()) {
				fatalf("expected ACME, got %s", body.Token.ShortString())
			}
			total.Add(total, &body.Amount)
			fmt.Printf("Deposited %s ACME (%x)\n", protocol.FormatBigAmount(&body.Amount, protocol.AcmePrecisionPower), res.TransactionHash)
		}
	}
	fmt.Println()
	fmt.Printf("Total %s ACME\n", protocol.FormatBigAmount(total, protocol.AcmePrecisionPower))
}

func as[T any](v any) *T {
	b, err := json.Marshal(v)
	check(err)
	u := new(T)
	check(json.Unmarshal(b, u))
	return u
}

func route(client *client.Client, account *url.URL) string {
	desc, err := client.Describe(context.Background())
	checkf(err, "query routing table")

	router, err := routing.NewStaticRouter(desc.Values.Routing, nil, nil)
	check(err)

	partition, err := router.RouteAccount(account)
	check(err)

	return partition
}

func getMajorBlockRange(client *client.Client, partition string, since time.Time) (uint64, uint64) {
	// Round to the nearest hour
	if since.Minute() > 0 || since.Hour() > 0 {
		fmt.Println("Rounding date to the nearest hour")
		since = since.Round(time.Hour)
	}

	// Hour must be 0:00 or 12:00 UTC
	since = since.UTC()
	if since.Hour() != 0 && since.Hour() != 12 {
		fatalf("date is not 0:00Z or 12:00Z")
	}

	// Calculate how many major blocks there have been
	const majorBlockSpan = 12 * time.Hour
	expectedNumBlocks := uint64(time.Since(since) / majorBlockSpan)

	// Determine how many major blocks there are
	anchors := protocol.PartitionUrl(partition).JoinPath(protocol.AnchorPool)
	req := new(api.GeneralQuery)
	req.Url = anchors.WithQuery("count=0").WithFragment("chain/major-block")
	mres := new(api.MultiResponse)
	check(client.RequestAPIv2(context.Background(), "query", req, mres))

	// Calculate range
	var start uint64
	count := expectedNumBlocks + flag.Backoff
	if mres.Total > count {
		start = mres.Total - count
	}

	// Query range
	req.Url = req.Url.WithQuery(fmt.Sprintf("start=%d&count=%d", start, count))
	mres = new(api.MultiResponse)
	check(client.RequestAPIv2(context.Background(), "query", req, mres))
	if len(mres.Items) == 0 {
		fatalf("major block query response is empty")
	}

	// Find first and lastMajor major blocks
	lastMajor := as[protocol.IndexEntry](mres.Items[len(mres.Items)-1])
	var firstMajor *protocol.IndexEntry
	for _, v := range mres.Items {
		block := as[protocol.IndexEntry](v)
		if block.BlockTime.UTC().Round(time.Hour).Equal(since) {
			firstMajor = block
			break
		}
	}
	if firstMajor == nil {
		fatalf("unable to find major block for %v, try --backoff %d", since, flag.Backoff+2)
	}

	ledger := protocol.PartitionUrl(partition).JoinPath(protocol.Ledger)
	req.Url = ledger.WithFragment(fmt.Sprintf("chain/root-index/%d", firstMajor.RootIndexIndex))
	cres := new(api.ChainQueryResponse)
	check(client.RequestAPIv2(context.Background(), "query", req, cres))
	firstMinor := as[protocol.IndexEntry](as[api.ChainEntry](cres.Data).Value)

	req.Url = ledger.WithFragment(fmt.Sprintf("chain/root-index/%d", lastMajor.RootIndexIndex))
	cres = new(api.ChainQueryResponse)
	check(client.RequestAPIv2(context.Background(), "query", req, cres))
	lastMinor := as[protocol.IndexEntry](as[api.ChainEntry](cres.Data).Value)

	return firstMinor.BlockIndex, lastMinor.BlockIndex
}

func findTransactionRange(client *client.Client, account *url.URL, firstBlock, lastBlock uint64) (firstTxn, lastTxn int) {
	req := new(api.GeneralQuery)
	req.Url = account.WithQuery("count=0").WithFragment("chain/main")
	mres := new(api.MultiResponse)
	check(client.RequestAPIv2(context.Background(), "query", req, mres))
	if mres.Total == 0 {
		fatalf("account has no transactions")
	}

	firstTxn = sort.Search(int(mres.Total), func(i int) bool {
		return getMainChainBlock(client, account, i) > firstBlock
	})
	if firstTxn == int(mres.Total) {
		fmt.Printf("No transactions exist on %v between %d and %d\n", account, firstBlock, lastBlock)
		os.Exit(1)
	}

	lastTxn = sort.Search(int(mres.Total)-firstTxn, func(i int) bool {
		return getMainChainBlock(client, account, firstTxn+i) > lastBlock
	})
	return firstTxn, lastTxn
}

func getMainChainBlock(client *client.Client, account *url.URL, i int) uint64 {
	req := new(api.GeneralQuery)
	req.Url = account.WithFragment(fmt.Sprintf("txn/%d", i))
	req.Prove = true
	res := new(api.TransactionQueryResponse)
	check(client.RequestAPIv2(context.Background(), "query", req, res))
	for _, r := range res.Receipts {
		if r.Chain == "main" {
			return r.LocalBlock
		}
	}

	fatalf("missing main chain receipt for %v#txn/%d", account, i)
	panic("not reachable")
}
