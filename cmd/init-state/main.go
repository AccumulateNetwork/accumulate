// Utility to initialize CometBFT state.db from a genesis.json file
// This is needed for blocksync when starting from genesis without InitChain
package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	sm "github.com/cometbft/cometbft/state"
	cmttypes "github.com/cometbft/cometbft/types"
	dbm "github.com/cometbft/cometbft-db"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <node-dir>\n", os.Args[0])
		os.Exit(1)
	}

	nodeDir := os.Args[1]
	genesisPath := filepath.Join(nodeDir, "config", "genesis.json")
	stateDBPath := filepath.Join(nodeDir, "data", "state.db")

	// Load genesis doc
	genDoc, err := cmttypes.GenesisDocFromFile(genesisPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading genesis: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Loaded genesis: chain_id=%s, initial_height=%d, validators=%d\n",
		genDoc.ChainID, genDoc.InitialHeight, len(genDoc.Validators))
	fmt.Printf("App hash: %s\n", hex.EncodeToString(genDoc.AppHash))

	// Validate and complete
	err = genDoc.ValidateAndComplete()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error validating genesis: %v\n", err)
		os.Exit(1)
	}

	// Create state from genesis
	state, err := sm.MakeGenesisState(genDoc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error making genesis state: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Genesis state: height=%d, validators=%d\n",
		state.LastBlockHeight, state.Validators.Size())

	// For blocksync, we need to set state to initial_height (usually 2 for mainnet)
	// This simulates the state after InitChain has run
	state.LastBlockHeight = genDoc.InitialHeight
	state.LastBlockTime = genDoc.GenesisTime
	state.AppHash = genDoc.AppHash
	state.LastResultsHash = cmttypes.NewResults(nil).Hash()
	state.LastValidators = state.Validators.Copy()

	fmt.Printf("Adjusted state: height=%d, app_hash=%s\n",
		state.LastBlockHeight, hex.EncodeToString(state.AppHash))

	// Remove existing state.db if present
	if _, err := os.Stat(stateDBPath); err == nil {
		if err := os.RemoveAll(stateDBPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error removing existing state.db: %v\n", err)
			os.Exit(1)
		}
	}

	// Open state.db
	dbDir := filepath.Join(nodeDir, "data")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating data dir: %v\n", err)
		os.Exit(1)
	}

	db, err := dbm.NewGoLevelDB("state", dbDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening state.db: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Create state store and save
	stateStore := sm.NewStore(db, sm.StoreOptions{
		DiscardABCIResponses: false,
	})

	err = stateStore.Save(state)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error saving state: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("State saved to %s\n", stateDBPath)
	fmt.Printf("CometBFT state initialized at height %d\n", state.LastBlockHeight)
}
