// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package message

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// Sequencer forwards [PrivateSequenceRequest]s to a [private.Sequencer].
type Sequencer struct {
	private.Sequencer
}

func (s Sequencer) methods() serviceMethodMap {
	typ, fn := makeServiceMethod(s.sequence)
	typ2, fn2 := makeServiceMethod(s.sequenceRange)
	typ3, fn3 := makeServiceMethod(s.majorHeaderRange)
	typ4, fn4 := makeServiceMethod(s.minorRootRange)
	return serviceMethodMap{typ: fn, typ2: fn2, typ3: fn3, typ4: fn4}
}

func (s Sequencer) sequence(c *call[*PrivateSequenceRequest]) {
	res, err := s.Sequencer.Sequence(c.context, c.params.Source, c.params.Destination, c.params.SequenceNumber, c.params.SequenceOptions)
	if err != nil {
		c.Write(&ErrorResponse{Error: errors.UnknownError.Wrap(err).(*errors.Error)})
		return
	}
	c.Write(&PrivateSequenceResponse{Value: res})
}

// sequenceRange serves a contiguous range under a single collection proof. The
// range extension is optional, so a sequencer that does not implement it says
// so rather than failing obscurely — the caller then falls back to per-message
// requests (#4087).
func (s Sequencer) sequenceRange(c *call[*PrivateSequenceRangeRequest]) {
	ranger, ok := s.Sequencer.(private.SequenceRanger)
	if !ok {
		c.Write(&ErrorResponse{Error: errors.NotAllowed.With("sequence range is not supported")})
		return
	}
	res, err := ranger.SequenceRange(c.context, c.params.Source, c.params.Destination, c.params.Start, c.params.End, c.params.SequenceOptions)
	if err != nil {
		c.Write(&ErrorResponse{Error: errors.UnknownError.Wrap(err).(*errors.Error)})
		return
	}
	c.Write(&PrivateSequenceRangeResponse{Value: res})
}

func (s Sequencer) majorHeaderRange(c *call[*PrivateMajorHeaderRangeRequest]) {
	ranger, ok := s.Sequencer.(private.MajorHeaderRanger)
	if !ok {
		c.Write(&ErrorResponse{Error: errors.NotAllowed.With("major header range is not supported")})
		return
	}
	res, err := ranger.MajorHeaderRange(c.context, c.params.Partition, c.params.Start, c.params.End, c.params.SequenceOptions)
	if err != nil {
		c.Write(&ErrorResponse{Error: errors.UnknownError.Wrap(err).(*errors.Error)})
		return
	}
	c.Write(&PrivateMajorHeaderRangeResponse{Value: res})
}

func (s Sequencer) minorRootRange(c *call[*PrivateMinorRootRangeRequest]) {
	ranger, ok := s.Sequencer.(private.MinorRootRanger)
	if !ok {
		c.Write(&ErrorResponse{Error: errors.NotAllowed.With("minor root range is not supported")})
		return
	}
	res, err := ranger.MinorRootRange(c.context, c.params.Partition, c.params.Since, c.params.Until, c.params.SequenceOptions)
	if err != nil {
		c.Write(&ErrorResponse{Error: errors.UnknownError.Wrap(err).(*errors.Error)})
		return
	}
	c.Write(&PrivateMinorRootRangeResponse{Value: res})
}

// PrivateClient is a binary message transport client for private API v3 services.
type PrivateClient AddressedClient

// Private returns a [PrivateClient].
func (c *Client) Private() private.Sequencer {
	return c.ForAddress(nil).Private()
}

// Private returns a [PrivateClient].
func (c AddressedClient) Private() private.Sequencer {
	return PrivateClient(c)
}

// Sequence implements [private.Sequencer.Sequence].
func (c PrivateClient) Sequence(ctx context.Context, src, dst *url.URL, num uint64, opts private.SequenceOptions) (*api.MessageRecord[messaging.Message], error) {
	req := &PrivateSequenceRequest{Source: src, Destination: dst, SequenceNumber: num, SequenceOptions: opts}
	return typedRequest[*PrivateSequenceResponse, *api.MessageRecord[messaging.Message]](AddressedClient(c), ctx, req)
}

// SequenceRange implements [private.SequenceRanger.SequenceRange].
func (c PrivateClient) SequenceRange(ctx context.Context, src, dst *url.URL, start, end uint64, opts private.SequenceOptions) ([]*api.MessageRecord[messaging.Message], error) {
	req := &PrivateSequenceRangeRequest{Source: src, Destination: dst, Start: start, End: end, SequenceOptions: opts}
	return typedRequest[*PrivateSequenceRangeResponse, []*api.MessageRecord[messaging.Message]](AddressedClient(c), ctx, req)
}

// MajorHeaderRange implements [private.MajorHeaderRanger.MajorHeaderRange].
func (c PrivateClient) MajorHeaderRange(ctx context.Context, partition *url.URL, start, end uint64, opts private.SequenceOptions) ([]*private.MajorHeaderRecord, error) {
	req := &PrivateMajorHeaderRangeRequest{Partition: partition, Start: start, End: end, SequenceOptions: opts}
	return typedRequest[*PrivateMajorHeaderRangeResponse, []*private.MajorHeaderRecord](AddressedClient(c), ctx, req)
}

func (r *PrivateSequenceRangeResponse) rval() []*api.MessageRecord[messaging.Message] { //nolint:unused
	return r.Value
}

// MinorRootRange implements [private.MinorRootRanger.MinorRootRange].
func (c PrivateClient) MinorRootRange(ctx context.Context, partition *url.URL, since, until uint64, opts private.SequenceOptions) (*private.MinorRootRecord, error) {
	req := &PrivateMinorRootRangeRequest{Partition: partition, Since: since, Until: until, SequenceOptions: opts}
	return typedRequest[*PrivateMinorRootRangeResponse, *private.MinorRootRecord](AddressedClient(c), ctx, req)
}

func (r *PrivateMajorHeaderRangeResponse) rval() []*private.MajorHeaderRecord { //nolint:unused
	return r.Value
}

func (r *PrivateMinorRootRangeResponse) rval() *private.MinorRootRecord { //nolint:unused
	return r.Value
}
