// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package encoding

import (
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage/memory"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package encoding ../../internal/database/snapshot/types.yml --include sigSection,Signature

func TestGenerateDbTestdata(t *testing.T) {
	t.Skip("Only run manually")

	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	logger := acctesting.NewTestLogger(t)
	store := memory.New(logger)
	db := database.New(store, logger)

	// Initialize
	sim := NewSim(t,
		func(partition string, _ int, logger log.Logger) database.Beginner {
			if strings.EqualFold(partition, protocol.Directory) {
				return database.OpenInMemory(logger)
			}
			return db
		},
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &protocol.TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: protocol.AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &protocol.TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: protocol.AcmeUrl()})

	// Execute
	st := sim.SubmitSuccessfully(MustBuild(t,
		build.Transaction().For(alice, "tokens").
			SendTokens(123, 0).To(bob, "tokens").
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey)))

	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	// Export
	f, err := os.Create("../testdata/database.json")
	require.NoError(t, err)
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	err = enc.Encode(store)
	require.NoError(t, err)

	f, err = os.Create("../testdata/database.snapshot")
	require.NoError(t, err)
	defer f.Close()
	batch := db.Begin(false)
	defer batch.Discard()
	_, err = snapshot.Collect(batch, new(snapshot.Header), f, snapshot.CollectOptions{
		Logger: logger,
	})
	require.NoError(t, err)
}

func TestDbEncoding(t *testing.T) {
	b, err := os.ReadFile("../testdata/database-v1.0.0.json")
	require.NoError(t, err)
	logger := acctesting.NewTestLogger(t)
	store := memory.New(logger)
	err = store.UnmarshalJSON(b)
	require.NoError(t, err)

	db := database.New(store, logger)
	batch := db.Begin(false)
	defer batch.Discard()
	buf := new(ioutil2.Buffer)
	_, err = snapshot.Collect(batch, new(snapshot.Header), buf, snapshot.CollectOptions{
		Logger: logger,
	})
	require.NoError(t, err)

	b, err = os.ReadFile("../testdata/database-v1.0.0.snapshot")
	require.NoError(t, err)

	h1, r1, err := snapshot.Open(ioutil2.NewBuffer(b))
	require.NoError(t, err)
	h2, r2, err := snapshot.Open(buf)
	require.NoError(t, err)

	require.Equal(t, h1, h2)

	for {
		var end1, end2 bool
		s1, err := r1.Next()
		if errors.Is(err, io.EOF) {
			end1 = true
		} else {
			require.NoError(t, err)
		}
		s2, err := r2.Next()
		if errors.Is(err, io.EOF) {
			end2 = true
		} else {
			require.NoError(t, err)
		}
		require.Equal(t, end1, end2, "Expected the same number of sections")
		if end1 {
			break
		}
		require.Equal(t, s1.Type(), s2.Type(), "Expected the same section types")
		require.Equal(t, s1.Offset(), s2.Offset(), "Expected the same offsets (%v)", s1.Type())
		assert.Equal(t, s1.Size(), s2.Size(), "Expected the same sizes (%v @ %d)", s1.Type(), s1.Offset())

		switch s1.Type() {
		case snapshot.SectionTypeSignatures:
			v1 := decodeSigSection(t, s1)
			v2 := decodeSigSection(t, s2)
			b1, err := json.MarshalIndent(v1.Signatures, "", "  ")
			require.NoError(t, err)
			b2, err := json.MarshalIndent(v2.Signatures, "", "  ")
			require.NoError(t, err)
			require.Equal(t, string(b1), string(b2), "Expected section content to match")
		default:
			b1 := dumpSection(t, s1)
			b2 := dumpSection(t, s2)
			require.Equal(t, b1, b2, "Expected section content to match")
		}
	}
}

func dumpSection(t *testing.T, s *snapshot.ReaderSection) []byte {
	rd, err := s.Open()
	require.NoError(t, err)
	b, err := io.ReadAll(rd)
	require.NoError(t, err)
	return b
}

func decodeSigSection(t *testing.T, s *snapshot.ReaderSection) *sigSection {
	rd, err := s.Open()
	require.NoError(t, err)
	sig := new(sigSection)
	require.NoError(t, sig.UnmarshalBinaryFrom(rd))
	return sig
}
