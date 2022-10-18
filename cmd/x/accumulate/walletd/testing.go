package walletd

import (
	"testing"
)

func InitTestDB(t *testing.T) {
	wallet = initDB(t.TempDir(), true)
}
