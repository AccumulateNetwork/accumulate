// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	apiv3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

// TestOffersService_ThePredicateIsPerServiceType — #4366, executor.md Sync
// step 5 and step 6 ("COMPLETE serves the services its committee membership
// gives it").
//
// The choke point is one — RegisterService — and what varies is the service
// type: submit and validate are offered only by committee membership, every
// other service by node state alone. #4336's per-node latch is vacuous for a
// follower that never joined (its nodeState is nil, so CanServeCurrent is
// Always{}), which is why the predicate has to take the service type.
func TestOffersService_ThePredicateIsPerServiceType(t *testing.T) {
	for _, c := range []struct {
		typ         apiv3.ServiceType
		inCommittee bool
		want        bool
	}{
		{apiv3.ServiceTypeSubmit, true, true},
		{apiv3.ServiceTypeSubmit, false, false},
		{apiv3.ServiceTypeValidate, true, true},
		{apiv3.ServiceTypeValidate, false, false},

		// A follower serves every read, in or out of the committee. The
		// readiness half of the predicate — a JOINING node advertising these
		// — is #4336's, and is not built here.
		{apiv3.ServiceTypeQuery, false, true},
		{apiv3.ServiceTypeConsensus, false, true},
		{apiv3.ServiceTypeNetwork, false, true},
		{apiv3.ServiceTypeEvent, false, true},
		{apiv3.ServiceTypeMetrics, false, true},
	} {
		in := c.inCommittee
		got := offersService(c.typ, func() bool { return in })
		require.Equalf(t, c.want, got, "%v with inCommittee=%v", c.typ, c.inCommittee)
	}

	// No predicate at all is the old behaviour: offered.
	require.True(t, offersService(apiv3.ServiceTypeSubmit, nil))
}

type stubSubmitter struct{ called *int }

func (s stubSubmitter) Submit(context.Context, *messaging.Envelope, apiv3.SubmitOptions) ([]*apiv3.Submission, error) {
	*s.called++
	return nil, errors.NotReady.With("stub")
}

func (s stubSubmitter) Validate(context.Context, *messaging.Envelope, apiv3.ValidateOptions) ([]*apiv3.Submission, error) {
	*s.called++
	return nil, errors.NotReady.With("stub")
}

// TestRegisterSubmitServices_AFollowerOffersNeither drives the daemon's own
// registration — the function cmd/accumulated/run/dagbft.go calls — against a
// real p2p node, and asks the node what it serves.
//
// By hand: the Instance, the stub submitter, and the membership answer. Not
// by hand: the service addresses, the registration, the advertise decision,
// the NodeInfo listing and the local dial resolution — all production.
func TestRegisterSubmitServices_AFollowerOffersNeither(t *testing.T) {
	newInstance := func(t *testing.T) *Instance {
		t.Helper()
		node, err := p2p.New(p2p.Options{Network: "offer-test"})
		require.NoError(t, err)
		t.Cleanup(func() { _ = node.Close() })
		return &Instance{
			logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
			services: ioc.Registry{},
			p2p:      node,
		}
	}

	listed := func(inst *Instance) []string {
		info, err := inst.p2p.Services().NodeInfo(context.Background(), apiv3.NodeInfoOptions{})
		require.NoError(t, err)
		var out []string
		for _, s := range info.Services {
			out = append(out, s.String())
		}
		return out
	}

	var calls int
	stub := stubSubmitter{&calls}

	t.Run("InCommittee", func(t *testing.T) {
		inst := newInstance(t)
		registerSubmitServices(inst, "BVN3", func() bool { return true }, stub, stub)
		require.Contains(t, listed(inst), apiv3.ServiceTypeSubmit.AddressFor("BVN3").String())
		require.Contains(t, listed(inst), apiv3.ServiceTypeValidate.AddressFor("BVN3").String())
	})

	t.Run("NotInCommittee", func(t *testing.T) {
		inst := newInstance(t)
		registerSubmitServices(inst, "BVN3", func() bool { return false }, stub, stub)
		require.NotContains(t, listed(inst), apiv3.ServiceTypeSubmit.AddressFor("BVN3").String(),
			"a node in no committee advertised submit")
		require.NotContains(t, listed(inst), apiv3.ServiceTypeValidate.AddressFor("BVN3").String(),
			"a node in no committee advertised validate")

		// And its own API does not resolve them locally: the dial falls
		// through to the network, which is how the client reaches a
		// validator without this node forwarding anything.
		require.False(t, inst.p2p.Offers(apiv3.ServiceTypeSubmit.AddressFor("BVN3")))
		require.False(t, inst.p2p.Offers(apiv3.ServiceTypeValidate.AddressFor("BVN3")))
	})
}
