package pmt

import "github.com/AccumulateNetwork/SMT/storage/database"

type Manager struct {
	DBManager *database.Manager
	Bpt       *BPT
}

func NewBPTManager(dbManager *database.Manager) *Manager {
	manager := new(Manager)
	manager.DBManager = dbManager
	manager.Bpt = new(BPT)
	return manager
}
