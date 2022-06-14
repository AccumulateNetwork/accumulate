package database

import (
	"fmt"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
)

const markPower = 8

func newMajorMinorIndexChain(store record.Store, key record.Key, namefmt, labelfmt string) *MajorMinorIndexChain {
	c := new(MajorMinorIndexChain)
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
