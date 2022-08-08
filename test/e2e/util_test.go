package e2e

import (
	"crypto/sha256"
	"sort"
	"strings"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var delivered = (*protocol.TransactionStatus).Delivered
var pending = (*protocol.TransactionStatus).Pending

type runTB[T any] interface {
	testing.TB
	Run(string, func(T)) bool
}

func RunSorted[Case any, TB runTB[TB]](t TB, cases map[string]Case, less func(a, b string) bool, run func(TB, Case)) {
	t.Helper()

	keys := make([]string, 0, len(cases))
	for key := range cases {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return less(keys[i], keys[j])
	})

	for _, key := range keys {
		t.Run(key, func(t TB) { run(t, cases[key]) })
	}
}

func Run[Case any, TB runTB[TB]](t TB, cases map[string]Case, run func(TB, Case)) {
	RunSorted(t, cases, func(a, b string) bool {
		return strings.Compare(a, b) < 0
	}, run)
}

func updateAccount[T protocol.Account](sim *simulator.Simulator, accountUrl *url.URL, fn func(account T)) {
	sim.UpdateAccount(accountUrl, func(account protocol.Account) {
		var typed T
		err := encoding.SetPtr(account, &typed)
		if err != nil {
			sim.Log(err)
			sim.FailNow()
		}

		fn(typed)
	})
}

func hash(b ...[]byte) []byte {
	h := sha256.New()
	for _, b := range b {
		_, _ = h.Write(b)
	}
	return h.Sum(nil)
}

func getPendingAsString(sim *simulator.Simulator, account *url.URL) []string {
	pending := simulator.GetAccountState[[]*url.TxID](sim, account, (*database.Account).Pending)

	var str []string
	for _, txid := range pending {
		str = append(str, txid.ShortString())
	}
	return str
}
