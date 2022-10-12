// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"strings"
	"testing"
	"text/tabwriter"
	"time"

	"github.com/stretchr/testify/require"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestBadOperatorPageUpdate(t *testing.T) {
	// Tests AC-3238

	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.Genesis(GenesisTime),
	)

	before := GetAccount[*KeyPage](t, sim.Database(Directory), DnUrl().JoinPath(Operators, "1"))

	// Execute
	st := sim.SubmitSuccessfully(MustBuild(t,
		build.Transaction().For(DnUrl(), Operators, "1").
			UpdateKeyPage().Add().Entry().Hash([32]byte{1}).FinishEntry().FinishOperation().
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(1).Signer(sim.SignWithNode(Directory, 0)).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(2).Signer(sim.SignWithNode(Directory, 1)).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(3).Signer(sim.SignWithNode(Directory, 2))))

	sim.StepUntil(
		Txn(st.TxID).Fails())

	// Verify the page did not change
	after := GetAccount[*KeyPage](t, sim.Database(Directory), DnUrl().JoinPath(Operators, "1"))
	require.Equal(t, before.AcceptThreshold, after.AcceptThreshold)
	require.Equal(t, len(before.Keys), len(after.Keys))
}

func TestBadOracleUpdate(t *testing.T) {
	// Tests AC-3238

	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.Genesis(GenesisTime),
	)

	before := GetAccount[*DataAccount](t, sim.Database(Directory), DnUrl().JoinPath(Oracle))
	v := new(AcmeOracle)
	require.NoError(t, v.UnmarshalBinary(before.Entry.GetData()[0]))

	// Execute
	st := sim.SubmitSuccessfully(MustBuild(t,
		build.Transaction().For(DnUrl(), Oracle).
			WriteData([]byte("foo")).ToState().
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(1).Signer(sim.SignWithNode(Directory, 0)).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(2).Signer(sim.SignWithNode(Directory, 1)).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(3).Signer(sim.SignWithNode(Directory, 2))))

	sim.StepUntil(
		Txn(st.TxID).Fails())

	// Verify the entry did not change
	after := GetAccount[*DataAccount](t, sim.Database(Directory), DnUrl().JoinPath(Oracle))
	v = new(AcmeOracle)
	require.NoError(t, v.UnmarshalBinary(after.Entry.GetData()[0]))
	require.True(t, before.Equal(after))
}

func TestTiming(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	batcher := sdktrace.NewBatchSpanProcessor(exp)
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(batcher))
	dtp := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(dtp) })
	otel.SetTracerProvider(tp)

	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 5, 10),
		// simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	// Do something
	st := sim.SubmitSuccessfully(MustBuild(t,
		build.Transaction().For(alice, "tokens").
			SendTokens(123, 0).To(bob, "tokens").
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey)))

	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	batcher.ForceFlush(context.Background())

	spans := map[trace.SpanID]*Span{}
	for _, s := range exp.GetSpans() {
		spans[s.SpanContext.SpanID()] = &Span{SpanStub: s}
	}

	roots := map[trace.SpanID]*Span{}
	for id, s := range spans {
		if !s.SpanStub.Parent.HasSpanID() {
			roots[id] = s
			continue
		}
		p := spans[s.SpanStub.Parent.SpanID()]
		p.Children = append(p.Children, s)
	}
	type BlockPart struct {
		Size     int64
		Count    int
		Part     string
		Duration time.Duration
	}
	type Block struct {
		Height int64
		Part   []*BlockPart
	}
	var blocks []*Block
	parts := map[string]int{
		"Directory": 0,
		"BVN0":      1,
		"BVN1":      2,
		"BVN2":      3,
		"BVN3":      4,
		"BVN4":      5,
	}
	for _, s := range roots {
		require.Equal(t, "Step", s.Name)
		for _, s := range s.Children {
			require.Equal(t, "Execute", s.Name)
			partition := getAttr(t, s, "Partition").AsString()
			for _, s := range s.Children {
				require.Equal(t, "Block", s.Name)
				height := getAttr(t, s, "Height").AsInt64()
				size := getAttr(t, s, "Size").AsInt64()

				for len(blocks) < int(height) {
					blocks = append(blocks, new(Block))
				}
				block := blocks[height-1]
				block.Height = height

				for len(block.Part) <= parts[partition] {
					block.Part = append(block.Part, new(BlockPart))
				}
				part := block.Part[parts[partition]]
				part.Size = size
				part.Part = partition
				part.Count++

				// Because of how the simulator handles multiple nodes, the
				// start and end times for the Block span are not useful. So
				// instead we must individually add up the time spent in
				// BeginBlock, DeliverTx, EndBlock, and Commit.
				for _, s := range s.Children {
					part.Duration += s.EndTime.Sub(s.StartTime)
				}
			}
		}
	}

	tw := tabwriter.NewWriter(os.Stdout, 1, 4, 1, ' ', 0)
	defer tw.Flush()
	fmt.Fprintf(tw, "Partition\tBlock\tSize\tDuration\n")
	for _, b := range blocks {
		for _, p := range b.Part {
			ms := fmt.Sprintf("%.03f", p.Duration.Seconds()*1000/float64(p.Count))
			i := strings.IndexRune(ms, '.')
			ms = strings.Repeat(" ", 4-i) + ms
			fmt.Fprintf(tw, "%s\t%d\t%d\t%s ms\n", p.Part, b.Height, p.Size, ms)
		}
	}
}

type Span struct {
	tracetest.SpanStub
	Children []*Span
}

func getAttr(t testing.TB, s *Span, name string) attribute.Value {
	for _, a := range s.Attributes {
		if string(a.Key) == name {
			return a.Value
		}
	}
	require.Failf(t, "Missing attribute", "Expected attribute %s", name)
	panic("not reached")
}
