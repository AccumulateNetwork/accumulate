// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package logging

import "github.com/cometbft/cometbft/libs/log"

// Logger is the standard logger interface used throughout the Accumulate
// codebase. It is currently a type alias for the CometBFT log.Logger to
// maintain backward compatibility. This alias allows packages to depend on
// internal/logging instead of importing CometBFT directly, making it easier
// to remove the CometBFT dependency in the future.
type Logger = log.Logger
