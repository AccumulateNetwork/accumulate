// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// RejoinSeed is written by `accumulated fastsync` next to a node's
// configuration and consumed once at the next startup (#4058). It carries
// the consensus position of the restored state — the round that committed
// the epoch block and the committee epoch — which exists nowhere else: the
// DAG is in-memory only, so a restarted node otherwise starts at round zero
// and wedges once the network's age exceeds the DAG GC depth.
type RejoinSeed struct {
	Partition string `json:"partition"`
	Block     uint64 `json:"block"`
	Round     uint64 `json:"round"`
	Epoch     uint64 `json:"epoch"`
}

// RejoinSeedName returns the seed file name for a partition.
func RejoinSeedName(partition string) string {
	return "fastsync-rejoin-" + strings.ToLower(partition) + ".json"
}

// WriteRejoinSeed writes a rejoin seed into the given directory.
func WriteRejoinSeed(dir string, seed *RejoinSeed) error {
	data, err := json.MarshalIndent(seed, "", "  ")
	if err != nil {
		return errors.InternalError.Wrap(err)
	}
	err = os.WriteFile(filepath.Join(dir, RejoinSeedName(seed.Partition)), data, 0600)
	return errors.UnknownError.Wrap(err)
}

// LoadRejoinSeed loads and consumes a partition's rejoin seed, if present.
// The file is renamed once loaded: the seed is only valid for the restart
// immediately after the fast sync — re-applying it later would jump
// consensus to a stale round, which wedges exactly like starting at zero.
func LoadRejoinSeed(dir, partition string) (*RejoinSeed, error) {
	path := filepath.Join(dir, RejoinSeedName(partition))
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		// Ok
	case os.IsNotExist(err):
		return nil, nil
	default:
		return nil, errors.UnknownError.Wrap(err)
	}

	seed := new(RejoinSeed)
	err = json.Unmarshal(data, seed)
	if err != nil {
		return nil, errors.EncodingError.WithFormat("parse rejoin seed: %w", err)
	}
	if !strings.EqualFold(seed.Partition, partition) {
		return nil, errors.Conflict.WithFormat("rejoin seed is for %s, not %s", seed.Partition, partition)
	}

	err = os.Rename(path, path+".applied")
	if err != nil {
		return nil, errors.UnknownError.WithFormat("consume rejoin seed: %w", err)
	}
	return seed, nil
}
