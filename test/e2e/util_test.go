package e2e

import (
	"crypto/sha256"
	"sort"
	"strings"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

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

type overrideExecutor struct {
	typ      protocol.TransactionType
	validate func(st *chain.StateManager, tx *chain.Delivery) error
	execute  func(st *chain.StateManager, tx *chain.Delivery) error
}

func (x *overrideExecutor) Type() protocol.TransactionType { return x.typ }

func (x *overrideExecutor) Execute(st *chain.StateManager, tx *chain.Delivery) (protocol.TransactionResult, error) {
	return nil, x.execute(st, tx)
}

func (x *overrideExecutor) Validate(st *chain.StateManager, tx *chain.Delivery) (protocol.TransactionResult, error) {
	return nil, x.validate(st, tx)
}

func hash(b ...[]byte) []byte {
	h := sha256.New()
	for _, b := range b {
		_, _ = h.Write(b)
	}
	return h.Sum(nil)
}
