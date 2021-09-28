package abci

import (
	"encoding/json"
	"fmt"

	dbm "github.com/tendermint/tm-db"
)

const stateKey = "stateKey"

type state struct {
	db      dbm.DB
	Size    int64  `json:"size"`
	Height  int64  `json:"height"`
	AppHash []byte `json:"app_hash"`
}

func loadState(db dbm.DB) (state, error) {
	var tmstate state
	tmstate.db = db
	stateBytes, err := db.Get([]byte(stateKey))
	if err != nil {
		return state{}, fmt.Errorf("failed to load state: %v", err)
	}
	if len(stateBytes) == 0 {
		return tmstate, nil
	}
	err = json.Unmarshal(stateBytes, &tmstate)
	if err != nil {
		return state{}, fmt.Errorf("failed to unmarshal state: %v", err)
	}
	return tmstate, nil
}

func saveState(state state) {
	stateBytes, err := json.Marshal(state)
	if err != nil {
		panic(err)
	}
	err = state.db.Set([]byte(stateKey), stateBytes)
	if err != nil {
		panic(err)
	}
}
