// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"golang.org/x/exp/slog"
)

func TestRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	c := &Config{
		Logging: &Logging{
			Format: "plain",
			Rules: []*LoggingRule{
				{Level: slog.LevelInfo},
			},
		},
		P2P: &P2P{
			Network: "DevNet",
			// BootstrapPeers: accumulate.BootstrapServers,
			Key: &PrivateKeySeed{Seed: record.NewKey("foo")},
		},
	}

	c.SaveTo("../../../.nodes/test.toml")

	ctx = logging.With(ctx, "test", t.Name())
	inst, err := Start(ctx, c)
	require.NoError(t, err)

	require.NoError(t, inst.Stop())
}
