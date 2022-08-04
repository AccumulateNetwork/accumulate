package walletd

import (
	"testing"
)

func InitTestDB(t *testing.T) {
	_ = initDB(t.TempDir(), true)
}
