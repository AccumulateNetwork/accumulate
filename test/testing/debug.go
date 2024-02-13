// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package testing

import "gitlab.com/accumulatenetwork/accumulate/internal/logging"

func EnableDebugFeatures() {
	logging.EnableDebugFeatures()
}

func DisableDebugFeatures() {
	logging.DisableDebugFeatures()
}
