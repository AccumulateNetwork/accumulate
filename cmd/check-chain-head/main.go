package main

import (
	"fmt"
	"os"

	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	bdg "gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Println("usage: check_chain_head <db> <kind> <url>")
		os.Exit(1)
	}
	bdg.ReadOnlyBadger = true

	var db *database.Database
	var err error
	switch os.Args[2] {
	case "leveldb":
		db, err = database.OpenLevelDB(os.Args[1], log.NewNopLogger())
	case "badger":
		db, err = database.OpenBadger(os.Args[1], log.NewNopLogger())
	}
	if err != nil {
		panic(err)
	}
	defer db.Close()
	batch := db.Begin(false)
	defer batch.Discard()
	u, err := url.Parse(os.Args[3])
	if err != nil {
		panic(err)
	}
	acc := batch.Account(u)
	chains, err := acc.Chains().Get()
	if err != nil {
		panic(err)
	}
	fmt.Printf("Chains() returns %d entries:\n", len(chains))
	for _, cm := range chains {
		ch, err := acc.GetChainByName(cm.Name)
		if err != nil {
			fmt.Printf("  %s: error %v\n", cm.Name, err)
			continue
		}
		st := ch.CurrentState()
		fmt.Printf("  %-20s type=%s count=%d hashListLen=%d pendingLen=%d anchor=%x\n",
			cm.Name, cm.Type, st.Count, len(st.HashList), len(st.Pending), st.Anchor())
	}
	// Try to access the underlying merkle managed name directly
	for _, name := range []string{"main", "signature", "scratch", "system", "anchor-sequence", "synthetic-sequence", "main-index", "signature-index"} {
		mgr, err := acc.ChainByName(name)
		if err != nil {
			continue
		}
		ch, err := mgr.Get()
		if err != nil {
			fmt.Printf("  ChainByName(%s) get err=%v\n", name, err)
			continue
		}
		st := ch.CurrentState()
		fmt.Printf("  ChainByName(%s) count=%d anchor=%x\n", name, st.Count, st.Anchor())
	}
}
