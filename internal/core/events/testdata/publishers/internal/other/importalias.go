// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package other

import ev "gitlab.com/accumulatenetwork/accumulate/internal/core/events"

func f(bus *ev.Bus) error { return bus.Publish(ev.WillChangeGlobals{}) }
