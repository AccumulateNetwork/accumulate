// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build !production
// +build !production

package testing

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
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
