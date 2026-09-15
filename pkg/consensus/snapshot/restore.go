// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

import (
	"context"
	"errors"
	"log/slog"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/dag"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// RestoreResult contains the result of a snapshot restore operation.
type RestoreResult struct {
	// Height is the block height restored to.
	Height uint64

	// Round is the consensus round restored to.
	Round types.Round

	// Committee is the restored committee.
	Committee *types.Committee

	// RestoredCertificates is the number of certificates restored to the DAG.
	RestoredCertificates int
}

// StateRestorer defines the interface for restoring application state.
type StateRestorer interface {
	// RestoreState restores application state from a snapshot.
	// The stateHash is used to verify the restored state matches the snapshot.
	RestoreState(ctx context.Context, height uint64, stateHash [32]byte) error
}

// Restorer handles snapshot restoration.
type Restorer struct {
	dag           *dag.DAG
	stateRestorer StateRestorer
}

// NewRestorer creates a new Restorer.
func NewRestorer(d *dag.DAG, sr StateRestorer) *Restorer {
	return &Restorer{
		dag:           d,
		stateRestorer: sr,
	}
}

// Restore restores consensus state from a snapshot.
func (r *Restorer) Restore(ctx context.Context, snapshot *Snapshot) (*RestoreResult, error) {
	if snapshot == nil {
		return nil, errors.New("snapshot is nil")
	}

	if err := snapshot.Validate(); err != nil {
		return nil, err
	}

	slog.Info("Restoring from snapshot",
		"height", snapshot.Height,
		"round", snapshot.Round,
		"certificates", len(snapshot.Certificates))

	// Restore application state first
	if r.stateRestorer != nil {
		if err := r.stateRestorer.RestoreState(ctx, snapshot.Height, snapshot.StateHash); err != nil {
			return nil, err
		}
	}

	// Restore certificates to DAG
	restoredCerts, err := r.restoreCertificates(snapshot)
	if err != nil {
		return nil, err
	}

	result := &RestoreResult{
		Height:               snapshot.Height,
		Round:                snapshot.Round,
		Committee:            snapshot.Committee.Clone(),
		RestoredCertificates: restoredCerts,
	}

	slog.Info("Snapshot restore complete",
		"height", result.Height,
		"round", result.Round,
		"restored_certificates", result.RestoredCertificates)

	return result, nil
}

// restoreCertificates restores certificates to the DAG in proper order.
func (r *Restorer) restoreCertificates(snapshot *Snapshot) (int, error) {
	if r.dag == nil || len(snapshot.Certificates) == 0 {
		return 0, nil
	}

	// Sort certificates by round to ensure parents are inserted first
	certs := make([]*types.Certificate, len(snapshot.Certificates))
	copy(certs, snapshot.Certificates)

	sort.Slice(certs, func(i, j int) bool {
		return certs[i].Round() < certs[j].Round()
	})

	// Group certificates by round for batch insertion
	certsByRound := make(map[types.Round][]*types.Certificate)
	for _, cert := range certs {
		round := cert.Round()
		certsByRound[round] = append(certsByRound[round], cert)
	}

	// Get sorted rounds
	rounds := make([]types.Round, 0, len(certsByRound))
	for round := range certsByRound {
		rounds = append(rounds, round)
	}
	sort.Slice(rounds, func(i, j int) bool {
		return rounds[i] < rounds[j]
	})

	restored := 0
	for _, round := range rounds {
		roundCerts := certsByRound[round]
		for _, cert := range roundCerts {
			var err error
			if round == 0 {
				err = r.dag.InsertGenesis(cert)
			} else {
				err = r.dag.Insert(cert)
			}

			if err != nil {
				// Log but continue - some certificates might already exist
				slog.Debug("Failed to insert certificate during restore",
					"round", round,
					"error", err)
				continue
			}
			restored++
		}
	}

	// Set the last commit round based on the snapshot
	r.dag.SetLastCommitRound(snapshot.Round)

	return restored, nil
}

// VerifySnapshot verifies that a snapshot is valid and can be restored.
func VerifySnapshot(snapshot *Snapshot, committee *types.Committee) error {
	if snapshot == nil {
		return errors.New("snapshot is nil")
	}

	if err := snapshot.Validate(); err != nil {
		return err
	}

	// Verify committee matches if provided
	if committee != nil {
		if snapshot.Committee.Epoch != committee.Epoch {
			return errors.New("snapshot committee epoch does not match")
		}
		if len(snapshot.Committee.Validators) != len(committee.Validators) {
			return errors.New("snapshot committee size does not match")
		}
	}

	// Verify certificates
	for i, cert := range snapshot.Certificates {
		if err := cert.Verify(snapshot.Committee); err != nil {
			slog.Debug("Certificate verification failed",
				"index", i,
				"round", cert.Round(),
				"error", err)
			return err
		}
	}

	return nil
}

// CanRestoreFrom checks if we can restore from the given snapshot.
func CanRestoreFrom(snapshot *Snapshot, currentHeight uint64) bool {
	if snapshot == nil {
		return false
	}

	// Can restore if snapshot is ahead of current state
	return snapshot.Height > currentHeight
}

// SelectBestSnapshot selects the best snapshot to restore from.
// It prefers the latest snapshot that has been verified.
func SelectBestSnapshot(snapshots []*Snapshot, committee *types.Committee) *Snapshot {
	if len(snapshots) == 0 {
		return nil
	}

	// Sort by height descending
	sorted := make([]*Snapshot, len(snapshots))
	copy(sorted, snapshots)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Height > sorted[j].Height
	})

	// Return the first verified snapshot
	for _, snap := range sorted {
		if err := VerifySnapshot(snap, committee); err == nil {
			return snap
		}
	}

	return nil
}

// RestoreOptions contains options for snapshot restoration.
type RestoreOptions struct {
	// VerifyState enables verification of the restored state hash.
	VerifyState bool

	// VerifyCertificates enables verification of all certificates.
	VerifyCertificates bool

	// SkipAppState skips restoring application state (useful for testing).
	SkipAppState bool
}

// DefaultRestoreOptions returns the default restore options.
func DefaultRestoreOptions() *RestoreOptions {
	return &RestoreOptions{
		VerifyState:        true,
		VerifyCertificates: true,
		SkipAppState:       false,
	}
}

// RestoreWithOptions restores from a snapshot with the given options.
func (r *Restorer) RestoreWithOptions(ctx context.Context, snapshot *Snapshot, opts *RestoreOptions) (*RestoreResult, error) {
	if opts == nil {
		opts = DefaultRestoreOptions()
	}

	if snapshot == nil {
		return nil, errors.New("snapshot is nil")
	}

	if err := snapshot.Validate(); err != nil {
		return nil, err
	}

	// Verify certificates if requested
	if opts.VerifyCertificates {
		for _, cert := range snapshot.Certificates {
			if err := cert.Verify(snapshot.Committee); err != nil {
				return nil, err
			}
		}
	}

	slog.Info("Restoring from snapshot with options",
		"height", snapshot.Height,
		"round", snapshot.Round,
		"verify_state", opts.VerifyState,
		"verify_certs", opts.VerifyCertificates,
		"skip_app_state", opts.SkipAppState)

	// Restore application state
	if !opts.SkipAppState && r.stateRestorer != nil {
		if err := r.stateRestorer.RestoreState(ctx, snapshot.Height, snapshot.StateHash); err != nil {
			return nil, err
		}
	}

	// Restore certificates to DAG
	restoredCerts, err := r.restoreCertificates(snapshot)
	if err != nil {
		return nil, err
	}

	return &RestoreResult{
		Height:               snapshot.Height,
		Round:                snapshot.Round,
		Committee:            snapshot.Committee.Clone(),
		RestoredCertificates: restoredCerts,
	}, nil
}
