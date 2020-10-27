package database

import (
	cfg "github.com/tendermint/tendermint/config"
	nm "github.com/tendermint/tendermint/node"
	dbm "github.com/tendermint/tm-db"
)

var AccountsDB   dbm.DB
var KvStoreDB   dbm.DB

func InitDBs(config *cfg.Config, dbProvider nm.DBProvider) (err error) {

	AccountsDB, err = dbProvider(&nm.DBContext{"accounts", config})
	if err != nil {
		return
	}


	KvStoreDB, err = dbProvider(&nm.DBContext{"kvStore", config})
	if err != nil {
		return
	}

	return
}