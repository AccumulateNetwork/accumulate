//go:build !production
// +build !production

package testing

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
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
