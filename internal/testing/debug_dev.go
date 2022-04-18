//go:build !production
// +build !production

package testing

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

func EnableDebugFeatures() {
	errors.EnableLocationTracking()
	storage.EnableKeyNameTracking()
}
