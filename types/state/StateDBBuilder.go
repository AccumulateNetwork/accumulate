package state

import (
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/tendermint/tendermint/libs/log"
)

type stateDBBuilder struct {
	debug  bool
	logger log.Logger
}

func NewStateDB() *stateDBBuilder {
	return new(stateDBBuilder)
}

func (sb *stateDBBuilder) WithDebug() *stateDBBuilder {
	sb.debug = true
	return sb
}

func (sb *stateDBBuilder) WithLogger(logger log.Logger) *stateDBBuilder {
	sb.logger = logger
	return sb
}

func (sb *stateDBBuilder) OpenInMemory() (*StateDB, error) {
	stateDB := new(StateDB)
	dbType := "memory"
	createStateLogger(sb, stateDB)
	err := stateDB.open(dbType, dbType, sb.logger)
	if err != nil {
		return nil, err
	}
	stateDB.init(sb.debug)
	return stateDB, nil
}

func (sb *stateDBBuilder) OpenFromFile(filePath string) (*StateDB, error) {
	stateDB := new(StateDB)
	dbType := "badger"
	createStateLogger(sb, stateDB)
	err := stateDB.open(dbType, filePath, sb.logger)
	if err != nil {
		return nil, err
	}

	stateDB.init(sb.debug)
	return stateDB, nil
}

func (sb *stateDBBuilder) LoadKeyValueDB(db storage.KeyValueDB) (*StateDB, error) {
	stateDB := new(StateDB)
	createStateLogger(sb, stateDB)
	err := stateDB.Load(db)
	stateDB.init(sb.debug)
	return stateDB, err
}

func createStateLogger(ib *stateDBBuilder, stateDB *StateDB) {
	if ib.logger != nil {
		stateDB.logger = ib.logger.With("module", "dbMgr")
	}
}
