package tendermint

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulated/blockchain/validator"
	"github.com/AccumulateNetwork/accumulated/config"
	dbm "github.com/tendermint/tm-db"
)

func NewApplication(config *config.Config, chainValidator *validator.ValidatorContext) (*AccumulatorVMApplication, error) {
	// Load state DB
	db, err := dbm.NewGoLevelDB("kvstore", config.RootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create GoLevelDB: %v", err)
	}

	// Load state
	state, err := loadState(db)
	if err != nil {
		return nil, err
	}

	// Initialize application
	app := &AccumulatorVMApplication{
		RetainBlocks: 1,     //only retain current block, we will manage our own states
		state:        state, //this will save the current state of the blockchain we can use if we need to restart
	}
	app.initialize(config)

	// Initialize validator node
	node := new(validator.Node)
	err = node.Initialize(config, app.Key.PrivKey.Bytes(), chainValidator)
	if err != nil {
		return nil, err
	}
	app.SetAccumulateNode(node)

	return app, nil
}
