// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package message

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// A Service is implements binary message transport for an API v3 service.
type Service interface {
	methods() serviceMethodMap
}

type serviceMethod func(c *call[Message])
type serviceMethodMap map[Type]serviceMethod

type msgPtr[T any] interface {
	*T
	Message
}

// makeServiceMethod returns a serviceMethod that constrains the request to a
// the given type and calls the given function.
func makeServiceMethod[T any, PT msgPtr[T]](fn func(*call[PT])) (Type, serviceMethod) {
	z := PT(new(T))
	return z.Type(),
		func(c *call[Message]) {
			// Check the Type
			if c.params.Type() != z.Type() {
				c.Write(&ErrorResponse{Error: errors.InternalError.WithFormat("bad message type: expected %v, got %v", z.Type(), c.params.Type())})
				return
			}

			// Check the type
			params, ok := c.params.(PT)
			if !ok {
				c.Write(&ErrorResponse{Error: errors.InternalError.WithFormat("bad message type: expected %T, got %T", z, c.params)})
				return
			}

			// Call it
			fn(&call[PT]{
				context: c.context,
				logger:  c.logger,
				stream:  c.stream,
				params:  params,
			})
		}

}

// call is a convenience struct to facilitate Service implementations.
type call[T Message] struct {
	context context.Context
	logger  logging.OptionalLogger
	stream  Stream
	params  T
}

// Write writes a message to the stream. If an error occurs, Write logs it and
// returns false.
func (c *call[T]) Write(msg Message) bool {
	err := c.stream.Write(msg)
	if err != nil {
		c.logger.Error("Unable to send response to peer", "error", err)
		return false
	}
	return true
}

// NodeService forwards [NodeStatusRequest]s to a [api.NodeService].
type NodeService struct {
	api.NodeService
}

func (s NodeService) methods() serviceMethodMap {
	typ, fn := makeServiceMethod(s.nodeStatus)
	return serviceMethodMap{typ: fn}
}

func (s NodeService) nodeStatus(c *call[*NodeStatusRequest]) {
	res, err := s.NodeService.NodeStatus(c.context, c.params.NodeStatusOptions)
	if err != nil {
		c.Write(&ErrorResponse{Error: errors.UnknownError.Wrap(err).(*errors.Error)})
		return
	}
	c.Write(&NodeStatusResponse{Value: res})
}

// NetworkService forwards [NetworkStatusRequest]s to a [api.NetworkService].
type NetworkService struct {
	api.NetworkService
}

func (s NetworkService) methods() serviceMethodMap {
	typ, fn := makeServiceMethod(s.networkStatus)
	return serviceMethodMap{typ: fn}
}

func (s NetworkService) networkStatus(c *call[*NetworkStatusRequest]) {
	res, err := s.NetworkService.NetworkStatus(c.context, c.params.NetworkStatusOptions)
	if err != nil {
		c.Write(&ErrorResponse{Error: errors.UnknownError.Wrap(err).(*errors.Error)})
		return
	}
	c.Write(&NetworkStatusResponse{Value: res})
}

// MetricsService forwards [MetricsRequest]s to a [api.MetricsService].
type MetricsService struct {
	api.MetricsService
}

func (s MetricsService) methods() serviceMethodMap {
	typ, fn := makeServiceMethod(s.metrics)
	return serviceMethodMap{typ: fn}
}

func (s MetricsService) metrics(c *call[*MetricsRequest]) {
	res, err := s.MetricsService.Metrics(c.context, c.params.MetricsOptions)
	if err != nil {
		c.Write(&ErrorResponse{Error: errors.UnknownError.Wrap(err).(*errors.Error)})
		return
	}
	c.Write(&MetricsResponse{Value: res})
}

// Querier forwards [QueryRequest]s to a [api.Querier].
type Querier struct {
	api.Querier
}

func (s Querier) methods() serviceMethodMap {
	typ, fn := makeServiceMethod(s.query)
	return serviceMethodMap{typ: fn}
}

func (s Querier) query(c *call[*QueryRequest]) {
	res, err := s.Querier.Query(c.context, c.params.Scope, c.params.Query)
	if err != nil {
		c.Write(&ErrorResponse{Error: errors.UnknownError.Wrap(err).(*errors.Error)})
		return
	}
	c.Write(&RecordResponse{Value: res})
}

// Submitter forwards [SubmitRequest]s to a [api.Submitter].
type Submitter struct {
	api.Submitter
}

func (s Submitter) methods() serviceMethodMap {
	typ, fn := makeServiceMethod(s.submit)
	return serviceMethodMap{typ: fn}
}

func (s Submitter) submit(c *call[*SubmitRequest]) {
	res, err := s.Submitter.Submit(c.context, c.params.Envelope, c.params.SubmitOptions)
	if err != nil {
		c.Write(&ErrorResponse{Error: errors.UnknownError.Wrap(err).(*errors.Error)})
		return
	}
	c.Write(&SubmitResponse{Value: res})
}

// Validator forwards [ValidateRequest]s to a [api.Validator].
type Validator struct {
	api.Validator
}

func (s Validator) methods() serviceMethodMap {
	typ, fn := makeServiceMethod(s.validate)
	return serviceMethodMap{typ: fn}
}

func (s Validator) validate(c *call[*ValidateRequest]) {
	res, err := s.Validator.Validate(c.context, c.params.Envelope, c.params.ValidateOptions)
	if err != nil {
		c.Write(&ErrorResponse{Error: errors.UnknownError.Wrap(err).(*errors.Error)})
		return
	}
	c.Write(&ValidateResponse{Value: res})
}
