// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package types

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCertificateVerifyRequiresParents: a certificate above round 0 names at
// least one parent, and no parent twice (#4591). Which parents, and whether
// they carry a quorum, is checked against the DAG when it is inserted.
func TestCertificateVerifyRequiresParents(t *testing.T) {
	var infos []ValidatorInfo
	var privs []ed25519.PrivateKey
	for i := 0; i < 4; i++ {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		infos = append(infos, ValidatorInfo{PublicKey: pub, Stake: 1})
		privs = append(privs, priv)
	}
	committee := NewCommittee(infos, 1)

	certify := func(round Round, parents []CertificateDigest) *Certificate {
		h := NewHeader(infos[0].PublicKey, round, 1, nil, parents)
		require.NoError(t, h.Sign(privs[0]))
		var sigs [][]byte
		var idx []uint16
		for i := 0; i < 3; i++ {
			v := NewVote(h.Digest(), round, 1, infos[i].PublicKey)
			require.NoError(t, v.Sign(privs[i]))
			sigs = append(sigs, v.Signature)
			idx = append(idx, uint16(i))
		}
		return NewCertificate(h, sigs, idx)
	}

	require.NoError(t, certify(0, nil).Verify(committee), "genesis has no parents")
	require.Error(t, certify(5, nil).Verify(committee), "a certificate above round 0 with no parents must not verify")
	parent := CertificateDigest{1}
	require.NoError(t, certify(5, []CertificateDigest{parent}).Verify(committee))
	require.Error(t, certify(5, []CertificateDigest{parent, parent}).Verify(committee), "a parent named twice must not verify")
}
