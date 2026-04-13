// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package genesis provides initialization logic for DAG-BFT consensus,
// including genesis certificate creation and DAG bootstrap.
package genesis

import (
	"crypto/ed25519"
	"encoding/binary"
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/dag"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// ValidatorInfo contains the public key and stake for a validator.
// This is used during genesis initialization to create the committee.
type ValidatorInfo struct {
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey
	Stake      uint64
}

// GenesisResult contains the results of genesis initialization.
type GenesisResult struct {
	// Committee is the initialized committee for epoch 1.
	Committee *types.Committee
	// GenesisCerts contains the genesis certificates for all validators.
	GenesisCerts []*types.Certificate
	// InitialRound is the round number to start at (always 1 after genesis).
	InitialRound types.Round
}

// InitGenesis creates genesis certificates for all validators and returns the
// initialized committee. Each validator creates a round-0 certificate with an
// empty payload and no parents. All validators sign all genesis certificates
// to establish initial quorum.
//
// The epoch is set to 1 for the genesis committee.
func InitGenesis(validators []ValidatorInfo) (*GenesisResult, error) {
	if len(validators) == 0 {
		return nil, errors.New("no validators provided")
	}

	// Validate all validators have required keys and stake
	for i, v := range validators {
		if len(v.PublicKey) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("validator %d has invalid public key size", i)
		}
		if len(v.PrivateKey) != ed25519.PrivateKeySize {
			return nil, fmt.Errorf("validator %d has invalid private key size", i)
		}
		if v.Stake == 0 {
			return nil, fmt.Errorf("validator %d has zero stake", i)
		}
		// Verify private key matches public key
		derivedPub := v.PrivateKey.Public().(ed25519.PublicKey)
		if !types.ValidatorsEqual(derivedPub, v.PublicKey) {
			return nil, fmt.Errorf("validator %d private key does not match public key", i)
		}
	}

	// Create committee from validators (epoch 1 is the genesis epoch)
	const genesisEpoch uint64 = 1
	committeeValidators := make([]types.ValidatorInfo, len(validators))
	for i, v := range validators {
		pubKey := make([]byte, ed25519.PublicKeySize)
		copy(pubKey, v.PublicKey)
		committeeValidators[i] = types.ValidatorInfo{
			PublicKey: pubKey,
			Stake:     v.Stake,
		}
	}
	committee := types.NewCommittee(committeeValidators, genesisEpoch)

	// Validate committee
	if err := committee.Validate(); err != nil {
		return nil, fmt.Errorf("invalid committee: %w", err)
	}

	// Create genesis certificates for each validator
	genesisCerts := make([]*types.Certificate, len(validators))
	for i, v := range validators {
		cert, err := createGenesisCertificate(v.PublicKey, v.PrivateKey, validators, committee)
		if err != nil {
			return nil, fmt.Errorf("create genesis cert for validator %d: %w", i, err)
		}
		genesisCerts[i] = cert
	}

	return &GenesisResult{
		Committee:    committee,
		GenesisCerts: genesisCerts,
		InitialRound: 1, // Always start at round 1 after genesis
	}, nil
}

// createGenesisCertificate creates a genesis certificate for a single validator.
// The certificate is at round 0 with an empty payload and no parents.
// All validators sign the certificate to establish initial agreement.
func createGenesisCertificate(
	author ed25519.PublicKey,
	authorKey ed25519.PrivateKey,
	validators []ValidatorInfo,
	committee *types.Committee,
) (*types.Certificate, error) {
	// Create genesis header (round 0, empty payload, no parents)
	header := types.NewHeader(author, 0, committee.Epoch, nil, nil)

	// Sign the header with the author's key
	if err := header.Sign(authorKey); err != nil {
		return nil, fmt.Errorf("sign header: %w", err)
	}

	// Collect signatures from all validators
	// Signatures must be over voteContent (headerDigest + round + epoch) to match certificate verification
	headerDigest := header.Digest()
	voteContent := make([]byte, 32+8+8)
	copy(voteContent[0:32], headerDigest[:])
	binary.BigEndian.PutUint64(voteContent[32:40], uint64(header.Round))
	binary.BigEndian.PutUint64(voteContent[40:48], header.Epoch)

	signatures := make([][]byte, len(validators))
	authorities := make([]uint16, len(validators))

	for i, v := range validators {
		// Sign the voteContent (matching certificate verification expectations)
		signatures[i] = ed25519.Sign(v.PrivateKey, voteContent)
		authorities[i] = uint16(i)
	}

	cert := types.NewCertificate(*header, signatures, authorities)

	// Verify the certificate is valid
	if err := cert.Verify(committee); err != nil {
		return nil, fmt.Errorf("verify genesis cert: %w", err)
	}

	return cert, nil
}

// BootstrapDAG inserts genesis certificates into the DAG and verifies
// all validators have genesis certs. Returns an error if any certificate
// fails to insert or if verification fails.
func BootstrapDAG(d *dag.DAG, committee *types.Committee, genesisCerts []*types.Certificate) error {
	if d == nil {
		return errors.New("DAG is nil")
	}
	if committee == nil {
		return errors.New("committee is nil")
	}
	if len(genesisCerts) == 0 {
		return errors.New("no genesis certificates provided")
	}

	// Verify we have a certificate for each committee member
	if len(genesisCerts) != committee.Len() {
		return fmt.Errorf("expected %d genesis certs, got %d", committee.Len(), len(genesisCerts))
	}

	// Track which validators have genesis certs
	seen := make(map[string]bool)

	// Insert each genesis certificate
	for i, cert := range genesisCerts {
		// Verify certificate
		if err := cert.Verify(committee); err != nil {
			return fmt.Errorf("verify genesis cert %d: %w", i, err)
		}

		// Check round is 0
		if cert.Round() != 0 {
			return fmt.Errorf("genesis cert %d has non-zero round %d", i, cert.Round())
		}

		// Check author is in committee
		authorKey := string(cert.Author())
		if seen[authorKey] {
			return fmt.Errorf("duplicate genesis cert for validator %d", i)
		}
		if !committee.ContainsValidator(cert.Author()) {
			return fmt.Errorf("genesis cert %d author not in committee", i)
		}
		seen[authorKey] = true

		// Insert into DAG
		if err := d.InsertGenesis(cert); err != nil {
			return fmt.Errorf("insert genesis cert %d: %w", i, err)
		}
	}

	// Verify all committee members have genesis certs
	for i := 0; i < committee.Len(); i++ {
		v := committee.GetValidator(uint16(i))
		if !seen[string(v.PublicKey)] {
			return fmt.Errorf("missing genesis cert for validator %d", i)
		}
	}

	// Verify DAG has quorum at round 0
	if !d.HasQuorum(0, committee) {
		return errors.New("DAG does not have quorum at genesis round")
	}

	return nil
}

// InitFromValidatorKeys is a convenience function that creates validators
// from private keys with equal stake, initializes genesis, and bootstraps
// the DAG. This is useful for testing and simple deployments.
func InitFromValidatorKeys(d *dag.DAG, keys []ed25519.PrivateKey, stake uint64) (*GenesisResult, error) {
	if len(keys) == 0 {
		return nil, errors.New("no validator keys provided")
	}
	if stake == 0 {
		stake = 1 // Default to unit stake
	}

	validators := make([]ValidatorInfo, len(keys))
	for i, key := range keys {
		validators[i] = ValidatorInfo{
			PublicKey:  key.Public().(ed25519.PublicKey),
			PrivateKey: key,
			Stake:      stake,
		}
	}

	result, err := InitGenesis(validators)
	if err != nil {
		return nil, err
	}

	if err := BootstrapDAG(d, result.Committee, result.GenesisCerts); err != nil {
		return nil, err
	}

	return result, nil
}
