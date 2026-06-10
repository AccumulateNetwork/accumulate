package main

import (
	"fmt"
	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	bdg "gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"os"
)

func main() {
	bdg.ReadOnlyBadger = true
	db, err := database.OpenLevelDB(os.Args[1], log.NewNopLogger())
	if err != nil {
		panic(err)
	}
	defer db.Close()
	batch := db.Begin(false)
	defer batch.Discard()
	u, _ := url.Parse(os.Args[2])
	acc := batch.Account(u)
	fmt.Println("Main:", func() string {
		m, e := acc.Main().Get()
		if e != nil {
			return "<err:" + e.Error() + ">"
		}
		return fmt.Sprintf("%T %v", m, m)
	}())
	if dir, e := acc.Directory().Get(); e == nil {
		for i, du := range dir {
			fmt.Printf("  dir[%d] = %s\n", i, du)
		}
	}
	if pend, e := acc.Pending().Get(); e == nil {
		for i, p := range pend {
			fmt.Printf("  pending[%d] = %s\n", i, p)
		}
	}
	chains, _ := acc.Chains().Get()
	for _, cm := range chains {
		ch, e := acc.GetChainByName(cm.Name)
		if e != nil {
			continue
		}
		st := ch.CurrentState()
		fmt.Printf("  chain %-20s count=%d anchor=%x\n", cm.Name, st.Count, st.Anchor())
	}
}
