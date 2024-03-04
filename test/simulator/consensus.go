// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"context"
	"runtime"

	"gitlab.com/accumulatenetwork/accumulate/test/simulator/consensus"
)

func (s *Simulator) newHub() consensus.Hub {
	h := consensus.NewSimpleHub(context.Background())
	for _, id := range s.partIDs {
		for _, n := range s.partitions[id].nodes {
			h.Register(n.consensus)
		}
	}
	return h
}

// Step executes a single simulator step
func (s *Simulator) Step() error {
	// TODO Care about s.deterministic

	// Execute a block
	err := s.newHub().Send(&consensus.StartBlock{})
	if err != nil {
		return err
	}

	// Wait for execution to complete
	err = s.tasks.Flush()

	// Give any parallel processes a chance to run
	runtime.Gosched()

	return err
}
