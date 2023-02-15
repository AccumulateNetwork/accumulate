package vdk

import (
	"testing"
)

func InitTestDB(t *testing.T) {
	wallet = nil
	DatabaseDir = t.TempDir()
	UseMemDB = true
	MustGetWallet()
}
