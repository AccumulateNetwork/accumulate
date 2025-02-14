// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

//lint:file-ignore ST1001 Don't care

import (
	"os"

	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v1/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v1/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

func InitFromSnapshot(t TB, db database.Beginner, exec *Executor, filename string) {
	t.Helper()

	f, err := os.Open(filename)
	require.NoError(tb{t}, err)
	defer f.Close()
	require.NoError(tb{t}, snapshot.FullRestore(db, f, exec.Logger, exec.Describe.PartitionUrl()))
	require.NoError(tb{t}, exec.Init(db))
}

func NormalizeEnvelope(t TB, envelope *messaging.Envelope) []*chain.Delivery {
	t.Helper()

	deliveries, err := chain.NormalizeEnvelope(envelope)
	require.NoError(tb{t}, err)
	return deliveries
}
