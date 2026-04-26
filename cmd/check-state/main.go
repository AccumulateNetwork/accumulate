package main

import (
	"encoding/hex"
	"fmt"
	"os"

	dbm "github.com/cometbft/cometbft-db"
	sm "github.com/cometbft/cometbft/state"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: check-state <path-to-data>")
		os.Exit(1)
	}
	db, err := dbm.NewGoLevelDB("state", os.Args[1])
	if err != nil {
		panic(err)
	}
	defer db.Close()

	fmt.Println("=== Raw keys in database ===")
	iter, err := db.Iterator(nil, nil)
	if err != nil {
		panic(err)
	}
	for ; iter.Valid(); iter.Next() {
		key := iter.Key()
		fmt.Printf("Key: %q (hex: %s), len(value): %d\n", string(key), hex.EncodeToString(key), len(iter.Value()))
	}
	iter.Close()

	fmt.Println("\n=== Using CometBFT state store ===")
	stateStore := sm.NewStore(db, sm.StoreOptions{})

	state, err := stateStore.Load()
	if err != nil {
		fmt.Printf("Failed to load state: %v\n", err)
		return
	}
	fmt.Printf("State loaded: LastBlockHeight=%d, InitialHeight=%d\n", state.LastBlockHeight, state.InitialHeight)
	fmt.Printf("State.Validators: %d\n", state.Validators.Size())
	fmt.Printf("State.LastValidators: %d\n", state.LastValidators.Size())
	fmt.Printf("State.NextValidators: %d\n", state.NextValidators.Size())

	// Try to load validators at specific heights
	heights := []int64{state.LastBlockHeight, state.LastBlockHeight + 1, state.LastBlockHeight - 1}
	for _, h := range heights {
		if h <= 0 {
			continue
		}
		vals, err := stateStore.LoadValidators(h)
		if err != nil {
			fmt.Printf("LoadValidators(%d): ERROR - %v\n", h, err)
		} else {
			fmt.Printf("LoadValidators(%d): OK - %d validators\n", h, vals.Size())
		}
	}
}
