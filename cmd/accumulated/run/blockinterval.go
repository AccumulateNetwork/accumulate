// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"log/slog"
	"time"

	dagconfig "gitlab.com/accumulatenetwork/accumulate/pkg/consensus/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
)

// resolveBlockInterval decides the cadence this node paces at, from what the
// network declares and what the node was configured with.
//
// A network runs at one block time and the network is what says so (#4267).
// Where the two disagree the node does not start: it is not silently honoured,
// because then the network runs at whatever the quorum's nodes happen to hold
// and nothing can detect the divergence; and it is not silently overridden,
// because an operator who set a value is entitled to be told their node is not
// running it.
//
// A network deployed before block time was recorded declares nothing. Those
// networks keep running on local configuration, which is what they already do.
func resolveBlockInterval(stated *encoding.Duration, globals *network.GlobalValues, partition string) (time.Duration, error) {
	var declared time.Duration
	if globals != nil && globals.Globals != nil {
		declared = globals.Globals.BlockInterval
	}

	switch {
	case declared > 0 && stated != nil && time.Duration(*stated) != declared:
		return 0, errors.Conflict.WithFormat(
			"block interval %v is configured on this node but the network runs at %v; "+
				"a network runs at one cadence, so remove the local setting or correct it",
			time.Duration(*stated), declared)

	case declared > 0:
		return declared, nil

	case stated != nil:
		slog.Warn("The network does not declare a block interval; pacing from local configuration",
			"partition", partition, "interval", time.Duration(*stated))
		return time.Duration(*stated), nil

	default:
		slog.Warn("The network does not declare a block interval and none is configured; using the default",
			"partition", partition, "interval", dagconfig.DefaultBlockInterval)
		return dagconfig.DefaultBlockInterval, nil
	}
}
