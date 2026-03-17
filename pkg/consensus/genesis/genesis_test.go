// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package genesis

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/dag"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

func generateKeys(n int) []ed25519.PrivateKey {
	keys := make([]ed25519.PrivateKey, n)
	for i := 0; i < n; i++ {
		_, keys[i], _ = ed25519.GenerateKey(rand.Reader)
	}
	return keys
}

func TestInitGenesis_Basic(t *testing.T) {
	keys := generateKeys(4)

	validators := make([]ValidatorInfo, len(keys))
	for i, key := range keys {
		validators[i] = ValidatorInfo{
			PublicKey:  key.Public().(ed25519.PublicKey),
			PrivateKey: key,
			Stake:      1,
		}
	}

	result, err := InitGenesis(validators)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify committee
	assert.Equal(t, len(keys), result.Committee.Len())
	assert.Equal(t, uint64(1), result.Committee.Epoch)

	// Verify genesis certs
	assert.Equal(t, len(keys), len(result.GenesisCerts))

	// All certs should be round 0
	for i, cert := range result.GenesisCerts {
		assert.Equal(t, types.Round(0), cert.Round(), "cert %d should be round 0", i)
		assert.NoError(t, cert.Verify(result.Committee), "cert %d should be valid", i)
	}

	// Initial round should be 1
	assert.Equal(t, types.Round(1), result.InitialRound)
}

func TestInitGenesis_EmptyValidators(t *testing.T) {
	_, err := InitGenesis(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no validators")

	_, err = InitGenesis([]ValidatorInfo{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no validators")
}

func TestInitGenesis_InvalidPublicKey(t *testing.T) {
	validators := []ValidatorInfo{
		{
			PublicKey:  []byte("short"),
			PrivateKey: make([]byte, ed25519.PrivateKeySize),
			Stake:      1,
		},
	}

	_, err := InitGenesis(validators)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid public key")
}

func TestInitGenesis_ZeroStake(t *testing.T) {
	_, privKey, _ := ed25519.GenerateKey(rand.Reader)
	pubKey := privKey.Public().(ed25519.PublicKey)

	validators := []ValidatorInfo{
		{
			PublicKey:  pubKey,
			PrivateKey: privKey,
			Stake:      0,
		},
	}

	_, err := InitGenesis(validators)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "zero stake")
}

func TestInitGenesis_KeyMismatch(t *testing.T) {
	_, privKey1, _ := ed25519.GenerateKey(rand.Reader)
	pubKey2, _, _ := ed25519.GenerateKey(rand.Reader)

	validators := []ValidatorInfo{
		{
			PublicKey:  pubKey2,           // Wrong public key
			PrivateKey: privKey1,          // Doesn't match
			Stake:      1,
		},
	}

	_, err := InitGenesis(validators)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not match")
}

func TestBootstrapDAG_Success(t *testing.T) {
	keys := generateKeys(4)

	validators := make([]ValidatorInfo, len(keys))
	for i, key := range keys {
		validators[i] = ValidatorInfo{
			PublicKey:  key.Public().(ed25519.PublicKey),
			PrivateKey: key,
			Stake:      1,
		}
	}

	result, err := InitGenesis(validators)
	require.NoError(t, err)

	d := dag.NewDAG(50)
	err = BootstrapDAG(d, result.Committee, result.GenesisCerts)
	require.NoError(t, err)

	// Verify all certs are in the DAG
	for _, cert := range result.GenesisCerts {
		assert.True(t, d.Contains(cert.Digest()), "DAG should contain genesis cert")
	}

	// Verify DAG has quorum at round 0
	assert.True(t, d.HasQuorum(0, result.Committee), "DAG should have quorum at round 0")
}

func TestBootstrapDAG_NilDAG(t *testing.T) {
	committee := types.NewCommittee(nil, 1)
	err := BootstrapDAG(nil, committee, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "DAG is nil")
}

func TestBootstrapDAG_NilCommittee(t *testing.T) {
	d := dag.NewDAG(50)
	err := BootstrapDAG(d, nil, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "committee is nil")
}

func TestBootstrapDAG_EmptyCerts(t *testing.T) {
	d := dag.NewDAG(50)
	committee := types.NewCommittee(nil, 1)
	err := BootstrapDAG(d, committee, []*types.Certificate{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no genesis certificates")
}

func TestInitFromValidatorKeys(t *testing.T) {
	keys := generateKeys(4)
	d := dag.NewDAG(50)

	result, err := InitFromValidatorKeys(d, keys, 1)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify committee
	assert.Equal(t, len(keys), result.Committee.Len())

	// Verify all certs are in the DAG
	for _, cert := range result.GenesisCerts {
		assert.True(t, d.Contains(cert.Digest()), "DAG should contain genesis cert")
	}
}

func TestInitFromValidatorKeys_DefaultStake(t *testing.T) {
	keys := generateKeys(2)
	d := dag.NewDAG(50)

	result, err := InitFromValidatorKeys(d, keys, 0) // 0 stake should default to 1
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify all validators have stake 1
	for i := 0; i < result.Committee.Len(); i++ {
		v := result.Committee.GetValidator(uint16(i))
		assert.Equal(t, uint64(1), v.Stake, "validator %d should have stake 1", i)
	}
}

func TestGenesisCertificates_AllValidatorsSigned(t *testing.T) {
	keys := generateKeys(4)

	validators := make([]ValidatorInfo, len(keys))
	for i, key := range keys {
		validators[i] = ValidatorInfo{
			PublicKey:  key.Public().(ed25519.PublicKey),
			PrivateKey: key,
			Stake:      1,
		}
	}

	result, err := InitGenesis(validators)
	require.NoError(t, err)

	// Each certificate should have signatures from all validators
	for i, cert := range result.GenesisCerts {
		assert.Equal(t, len(keys), len(cert.Signatures),
			"cert %d should have %d signatures", i, len(keys))
		assert.Equal(t, len(keys), len(cert.SignedAuthorities),
			"cert %d should have %d authorities", i, len(keys))

		// Verify total stake meets quorum
		totalStake := cert.TotalStake(result.Committee)
		assert.True(t, result.Committee.HasQuorum(totalStake),
			"cert %d should have quorum stake", i)
	}
}

func TestGenesisCertificates_UniqueAuthors(t *testing.T) {
	keys := generateKeys(4)

	validators := make([]ValidatorInfo, len(keys))
	for i, key := range keys {
		validators[i] = ValidatorInfo{
			PublicKey:  key.Public().(ed25519.PublicKey),
			PrivateKey: key,
			Stake:      1,
		}
	}

	result, err := InitGenesis(validators)
	require.NoError(t, err)

	// Each certificate should have a unique author
	seen := make(map[string]bool)
	for i, cert := range result.GenesisCerts {
		authorKey := string(cert.Author())
		assert.False(t, seen[authorKey], "cert %d has duplicate author", i)
		seen[authorKey] = true
	}
}

func TestGenesisCertificates_EmptyPayload(t *testing.T) {
	keys := generateKeys(2)

	validators := make([]ValidatorInfo, len(keys))
	for i, key := range keys {
		validators[i] = ValidatorInfo{
			PublicKey:  key.Public().(ed25519.PublicKey),
			PrivateKey: key,
			Stake:      1,
		}
	}

	result, err := InitGenesis(validators)
	require.NoError(t, err)

	// Genesis certs should have empty payloads
	for i, cert := range result.GenesisCerts {
		assert.Empty(t, cert.Header.Payload, "cert %d should have empty payload", i)
	}
}

func TestGenesisCertificates_NoParents(t *testing.T) {
	keys := generateKeys(2)

	validators := make([]ValidatorInfo, len(keys))
	for i, key := range keys {
		validators[i] = ValidatorInfo{
			PublicKey:  key.Public().(ed25519.PublicKey),
			PrivateKey: key,
			Stake:      1,
		}
	}

	result, err := InitGenesis(validators)
	require.NoError(t, err)

	// Genesis certs should have no parents
	for i, cert := range result.GenesisCerts {
		assert.Empty(t, cert.Parents(), "cert %d should have no parents", i)
	}
}

func TestInitGenesis_VariableStakes(t *testing.T) {
	keys := generateKeys(4)

	// Different stakes for each validator
	stakes := []uint64{10, 20, 30, 40}
	validators := make([]ValidatorInfo, len(keys))
	for i, key := range keys {
		validators[i] = ValidatorInfo{
			PublicKey:  key.Public().(ed25519.PublicKey),
			PrivateKey: key,
			Stake:      stakes[i],
		}
	}

	result, err := InitGenesis(validators)
	require.NoError(t, err)

	// Verify stakes are correct
	for i := 0; i < result.Committee.Len(); i++ {
		v := result.Committee.GetValidator(uint16(i))
		assert.Equal(t, stakes[i], v.Stake, "validator %d should have stake %d", i, stakes[i])
	}

	// Total stake should be sum of all stakes
	expectedTotal := uint64(10 + 20 + 30 + 40)
	assert.Equal(t, expectedTotal, result.Committee.TotalStake())
}

func TestBootstrapDAG_RoundAfterGenesis(t *testing.T) {
	keys := generateKeys(4)

	validators := make([]ValidatorInfo, len(keys))
	for i, key := range keys {
		validators[i] = ValidatorInfo{
			PublicKey:  key.Public().(ed25519.PublicKey),
			PrivateKey: key,
			Stake:      1,
		}
	}

	result, err := InitGenesis(validators)
	require.NoError(t, err)

	d := dag.NewDAG(50)
	err = BootstrapDAG(d, result.Committee, result.GenesisCerts)
	require.NoError(t, err)

	// DAG latest round should be 0 (genesis)
	assert.Equal(t, types.Round(0), d.LatestRound())

	// But we should start consensus at round 1
	assert.Equal(t, types.Round(1), result.InitialRound)
}
