package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"

	dbm "github.com/cometbft/cometbft-db"
	sm "github.com/cometbft/cometbft/state"
)

type GenesisValidator struct {
	Address string            `json:"address"`
	PubKey  map[string]string `json:"pub_key"`
	Power   string            `json:"power"`
	Name    string            `json:"name,omitempty"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: extract-validators <path-to-data>")
		os.Exit(1)
	}
	db, err := dbm.NewGoLevelDB("state", os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	stateStore := sm.NewStore(db, sm.StoreOptions{})
	state, err := stateStore.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load state: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "State: LastBlockHeight=%d, InitialHeight=%d, Validators=%d\n",
		state.LastBlockHeight, state.InitialHeight, state.Validators.Size())

	validators := make([]GenesisValidator, 0, state.Validators.Size())
	for _, v := range state.Validators.Validators {
		gv := GenesisValidator{
			Address: fmt.Sprintf("%X", v.Address),
			PubKey: map[string]string{
				"type":  "tendermint/PubKeyEd25519",
				"value": base64.StdEncoding.EncodeToString(v.PubKey.Bytes()),
			},
			Power: fmt.Sprintf("%d", v.VotingPower),
		}
		validators = append(validators, gv)
	}

	output, _ := json.MarshalIndent(validators, "", "  ")
	fmt.Println(string(output))
}
