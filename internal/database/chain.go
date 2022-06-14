package database

import (
	"fmt"
	"strings"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
)

const markPower = 8

func newMajorMinorIndexChain(logger log.Logger, store record.Store, key record.Key, namefmt, labelfmt string) *MajorMinorIndexChain {
	c := new(MajorMinorIndexChain)
	c.logger.L = logger
	c.store = store
	c.key = key
	if strings.ContainsRune(namefmt, '%') {
		c.name = fmt.Sprintf(namefmt, key...)
	} else {
		c.name = namefmt
	}
	c.label = fmt.Sprintf(labelfmt, key...)
	return c
}

func tryResolveChain[T interface {
	resolveChain(name string) (*managed.Chain, bool)
}](chainPtr **managed.Chain, okPtr *bool, name, prefix string, getField func() T) {
	if *okPtr || !strings.HasPrefix(name, prefix) {
		return
	}

	chain, ok := getField().resolveChain(name[len(prefix):])
	if ok {
		*chainPtr, *okPtr = chain, true
	}
}

func tryResolveChainParam(chainPtr **managed.Chain, okPtr *bool, name, prefix string, expectLen int, resolve func([]string, string) (*managed.Chain, bool)) {
	if *okPtr || !strings.HasPrefix(name, prefix) {
		return
	}

	name = name[len(prefix):]
	i := strings.Index(name, ")")
	if i < 0 {
		return
	}

	params := strings.Split(name[:i], ",")
	name = name[i+1:]
	if len(params) != expectLen {
		return
	}

	if strings.HasPrefix(name, "-") {
		name = name[1:]
	}
	chain, ok := resolve(params, name)
	if ok {
		*chainPtr, *okPtr = chain, true
	}
}

func parseChainParam[T any](ok *bool, s string, parse func(string) (T, error)) T {
	if !*ok {
		return zero[T]()
	}

	v, err := parse(s)
	if err == nil {
		return v
	}

	*ok = false
	return zero[T]()
}
