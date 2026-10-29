// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package other

import "gitlab.com/accumulatenetwork/accumulate/internal/core/events"

func forward(dst *events.Bus, e events.WillChangeGlobals) error {
	var y = e
	var z = y
	return dst.Publish(z)
}
