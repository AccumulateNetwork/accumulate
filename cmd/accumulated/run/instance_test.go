// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"golang.org/x/exp/slog"
)

func TestMainNetHttp(t *testing.T) {
	t.Skip("Manual")

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	c := &Config{
		Network: "MainNet",
		Logging: &Logging{
			Format: "plain",
			Rules: []*LoggingRule{
				{Level: slog.LevelInfo},
			},
		},
		P2P: &P2P{
			Key: &PrivateKeySeed{Seed: record.NewKey(t.Name())},
		},
		Configurations: []Configuration{
			&GatewayConfiguration{},
		},
	}

	ctx = logging.With(ctx, "test", t.Name())
	inst, err := Start(ctx, c)
	require.NoError(t, err)

	time.Sleep(time.Hour)

	require.NoError(t, inst.Stop())
}
