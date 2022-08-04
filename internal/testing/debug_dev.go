////go:build !production
//// +build !production

package testing

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/memory"
)

func EnableDebugFeatures() {
	errors.EnableLocationTracking()
	storage.EnableKeyNameTracking()
	memory.EnableLogWrites()
}

func DisableDebugFeatures() {
	errors.DisableLocationTracking()
	storage.DisableKeyNameTracking()
	memory.DisableLogWrites()
}
