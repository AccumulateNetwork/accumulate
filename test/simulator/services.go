// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"context"
	"crypto/sha256"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// simService implements API v3.
type simService Simulator

// Private returns the private sequencer service.
func (s *simService) Private() private.Sequencer { return s }

func (s *simService) Faucet(ctx context.Context, account *url.URL, opts api.FaucetOptions) (*api.Submission, error) {
	return nil, errors.NotAllowed.With("not implemented")
}

// ConsensusStatus finds the specified node and returns its ConsensusStatus.
func (s *simService) ConsensusStatus(ctx context.Context, opts api.ConsensusStatusOptions) (*api.ConsensusStatus, error) {
	if opts.NodeID == "" {
		return nil, errors.BadRequest.WithFormat("node ID is missing")
	}
	id, err := peer.Decode(opts.NodeID)
	if err != nil {
		return nil, errors.BadRequest.WithFormat("invalid peer ID: %w", err)
	}
	for _, p := range s.partitions {
		for _, n := range p.nodes {
			sk, err := crypto.UnmarshalEd25519PrivateKey(n.nodeKey)
			if err != nil {
				continue
			}
			if id.MatchesPrivateKey(sk) {
				return (*nodeService)(n).ConsensusStatus(ctx, opts)
			}
		}
	}
	return nil, errors.NotFound.WithFormat("node %s not found", id)
}

// NetworkStatus implements pkg/api/v3.NetworkService.
func (s *simService) NetworkStatus(ctx context.Context, opts api.NetworkStatusOptions) (*api.NetworkStatus, error) {
	p, ok := s.partitions[opts.Partition]
	if !ok {
		return nil, errors.NotFound.WithFormat("%q is not a partition", opts.Partition)
	}
	return (*nodeService)(p.nodes[0]).NetworkStatus(ctx, opts)
}

// Metrics implements pkg/api/v3.MetricsService.
func (s *simService) Metrics(ctx context.Context, opts api.MetricsOptions) (*api.Metrics, error) {
	p, ok := s.partitions[opts.Partition]
	if !ok {
		return nil, errors.NotFound.WithFormat("%q is not a partition", opts.Partition)
	}
	return (*nodeService)(p.nodes[0]).Metrics(ctx, opts)
}

// Query routes the scope to a partition and calls Query on the first node of
// that partition, returning the result.
func (s *simService) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	r, err := s.queryFanout(ctx, scope, query)
	if r != nil || err != nil {
		return r, err
	}

	part, err := s.router.RouteAccount(scope)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return (*nodeService)(s.partitions[part].nodes[0]).Query(ctx, scope, query)
}

func (s *simService) queryFanout(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	// If the request is a transaction hash search query request, fan it out
	if scope == nil || !protocol.IsUnknown(scope) {
		return nil, nil
	}
	if _, ok := query.(*api.MessageHashSearchQuery); !ok {
		return nil, nil
	}

	records := new(api.RecordRange[api.Record])
	for _, p := range s.partitions {
		if p.ID == "BVN1" {
			print("")
		}
		r, err := (*nodeService)(p.nodes[0]).Query(ctx, scope, query)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		rr, ok := r.(*api.RecordRange[api.Record])
		if !ok {
			return nil, errors.InternalError.WithFormat("expected %v, got %v", api.RecordTypeRange, r.RecordType())
		}
		records.Records = append(records.Records, rr.Records...)
	}
	records.Total = uint64(len(records.Records))
	return records, nil
}

// Submit routes the envelope to a partition and calls Submit on the first node
// of that partition, returning the result.
func (s *simService) Submit(ctx context.Context, envelope *messaging.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
	part, err := s.router.Route(envelope)
	if err != nil {
		return nil, err
	}
	return (*nodeService)(s.partitions[part].nodes[0]).Submit(ctx, envelope, opts)
}

// Validate routes the envelope to a partition and calls Validate on the first
// node of that partition, returning the result.
func (s *simService) Validate(ctx context.Context, envelope *messaging.Envelope, opts api.ValidateOptions) ([]*api.Submission, error) {
	part, err := s.router.Route(envelope)
	if err != nil {
		return nil, err
	}
	return (*nodeService)(s.partitions[part].nodes[0]).Validate(ctx, envelope, opts)
}

// Subscribe implements [api.EventService].
func (s *simService) Subscribe(ctx context.Context, opts api.SubscribeOptions) (<-chan api.Event, error) {
	if opts.Partition != "" {
		p, ok := s.partitions[opts.Partition]
		if !ok {
			return nil, errors.NotFound.WithFormat("%q is not a partition", opts.Partition)
		}
		return (*nodeService)(p.nodes[0]).Subscribe(ctx, opts)
	}
	if opts.Account != nil {
		part, err := s.router.RouteAccount(opts.Account)
		if err != nil {
			return nil, err
		}
		return (*nodeService)(s.partitions[part].nodes[0]).Subscribe(ctx, opts)
	}
	return nil, errors.BadRequest.With("either partition or account is required")
}

// Sequence routes the source to a partition and calls Sequence on the first
// node of that partition, returning the result.
func (s *simService) Sequence(ctx context.Context, src, dst *url.URL, num uint64) (*api.MessageRecord[messaging.Message], error) {
	part, err := s.router.RouteAccount(src)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return (*nodeService)(s.partitions[part].nodes[0]).Private().Sequence(ctx, src, dst, num)
}

// nodeService implements API v3.
type nodeService Node

// Private returns the private sequencer service.
func (s *nodeService) Private() private.Sequencer { return s.seqSvc }

func (s *nodeService) Faucet(ctx context.Context, account *url.URL, opts api.FaucetOptions) (*api.Submission, error) {
	return nil, errors.NotAllowed.With("not implemented")
}

// ConsensusStatus implements [api.ConsensusService].
func (s *nodeService) ConsensusStatus(ctx context.Context, opts api.ConsensusStatusOptions) (*api.ConsensusStatus, error) {
	return &api.ConsensusStatus{
		Ok: true,
		LastBlock: &api.LastBlock{
			Height: int64(s.partition.blockIndex),
			Time:   s.partition.blockTime,
			// TODO: chain root, state root
		},
		NodeKeyHash:      sha256.Sum256(s.nodeKey[32:]),
		ValidatorKeyHash: sha256.Sum256(s.privValKey[32:]),
		PartitionID:      s.partition.ID,
		PartitionType:    s.partition.Type,
	}, nil
}

// NetworkStatus implements [api.NetworkService].
func (s *nodeService) NetworkStatus(ctx context.Context, opts api.NetworkStatusOptions) (*api.NetworkStatus, error) {
	v, ok := s.globals.Load().(*core.GlobalValues)
	if !ok {
		return nil, errors.NotReady
	}
	return &api.NetworkStatus{
		Oracle:  v.Oracle,
		Network: v.Network,
		Globals: v.Globals,
		Routing: v.Routing,
	}, nil
}

// Metrics implements [api.MetricsService].
func (s *nodeService) Metrics(ctx context.Context, opts api.MetricsOptions) (*api.Metrics, error) {
	return nil, errors.NotAllowed
}

// Query implements [api.Querier].
func (s *nodeService) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	// Copy to avoid changing the caller's values
	query = api.CopyQuery(query)

	r, err := s.querySvc.Query(ctx, scope, query)
	if err != nil {
		return nil, err
	}

	// Force despecialization of generic types
	b, err := r.MarshalBinary()
	if err != nil {
		return nil, errors.InternalError.Wrap(err)
	}
	r, err = api.UnmarshalRecord(b)
	if err != nil {
		return nil, errors.InternalError.Wrap(err)
	}
	return r, nil
}

// Submit implements [api.Submitter].
func (s *nodeService) Submit(ctx context.Context, envelope *messaging.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
	return s.submit(envelope, false)
}

// Validate implements [api.Validator].
func (s *nodeService) Validate(ctx context.Context, envelope *messaging.Envelope, opts api.ValidateOptions) ([]*api.Submission, error) {
	return s.submit(envelope, true)
}

// Subscribe implements [api.EventService].
func (s *nodeService) Subscribe(ctx context.Context, opts api.SubscribeOptions) (<-chan api.Event, error) {
	return s.eventSvc.Subscribe(ctx, opts)
}

func (s *nodeService) submit(envelope *messaging.Envelope, pretend bool) ([]*api.Submission, error) {
	st, err := s.partition.Submit(envelope, pretend)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	subs := make([]*api.Submission, len(st))
	for i, st := range st {
		// Create an api.Submission
		subs[i] = new(api.Submission)
		subs[i].Status = st
		subs[i].Success = st.Code.Success()
		if st.Error != nil {
			subs[i].Message = st.Error.Message
		}
	}

	return subs, nil
}
