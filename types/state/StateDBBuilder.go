package state

import (
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/tendermint/tendermint/libs/log"
)

type stateDbBuilder struct {
	debug  bool
	logger log.Logger
}

func NewStateDB() *stateDbBuilder {
	return new(stateDbBuilder)
}

func (sb *stateDbBuilder) WithDebug() *stateDbBuilder {
	sb.debug = true
	return sb
}

func (sb *stateDbBuilder) WithLogger(logger log.Logger) *stateDbBuilder {
	sb.logger = logger
	return sb
}

func (sb *stateDbBuilder) OpenInMemory() (*StateDB, error) {
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

func (sb *stateDbBuilder) OpenFromFile(filePath string) (*StateDB, error) {
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

func (sb *stateDbBuilder) LoadKeyValueDB(db storage.KeyValueDB) (*StateDB, error) {
	stateDB := new(StateDB)
	createStateLogger(sb, stateDB)
	err := stateDB.Load(db)
	stateDB.init(sb.debug)
	return stateDB, err
}

func createStateLogger(ib *stateDbBuilder, stateDB *StateDB) {
	if ib.logger != nil {
		stateDB.logger = ib.logger.With("module", "dbMgr")
	}
}
