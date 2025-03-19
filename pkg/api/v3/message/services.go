// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package message

import (
	"context"
	"fmt"
)

// ServiceClient defines the interface for service-related operations
type ServiceClient interface {
	NodeInfo(ctx context.Context, opts *NodeInfoRequest) (*NodeInfoResponse, error)
	FindService(ctx context.Context, opts *FindServiceRequest) (*FindServiceResponse, error)
	ConsensusStatus(ctx context.Context, opts *ConsensusStatusRequest) (*ConsensusStatusResponse, error)
	NetworkStatus(ctx context.Context, opts *NetworkStatusRequest) (*NetworkStatusResponse, error)
	Query(ctx context.Context, opts *QueryRequest) (*QueryResponse, error)
}

// NodeInfoRequest represents a request for node information
type NodeInfoRequest struct {
	Options interface{} `json:"options,omitempty"`
}

// NodeInfoResponse represents a response containing node information
type NodeInfoResponse struct {
	Info interface{} `json:"info,omitempty"`
}

// FindServiceRequest represents a request to find a service
type FindServiceRequest struct {
	Options interface{} `json:"options,omitempty"`
}

// FindServiceResponse represents a response containing service results
type FindServiceResponse struct {
	Services []interface{} `json:"services,omitempty"`
}

// ConsensusStatusRequest represents a request for consensus status
type ConsensusStatusRequest struct {
	Options interface{} `json:"options,omitempty"`
}

// ConsensusStatusResponse represents a response containing consensus status
type ConsensusStatusResponse struct {
	Status interface{} `json:"status,omitempty"`
}

// NetworkStatusRequest represents a request for network status
type NetworkStatusRequest struct {
	Options interface{} `json:"options,omitempty"`
}

// NetworkStatusResponse represents a response containing network status
type NetworkStatusResponse struct {
	Status interface{} `json:"status,omitempty"`
}

// QueryRequest represents a query request
type QueryRequest struct {
	Query interface{} `json:"query,omitempty"`
}

// QueryResponse represents a query response
type QueryResponse struct {
	Result interface{} `json:"result,omitempty"`
}

// Type implementations for request messages
func (r *NodeInfoRequest) Type() uint32         { return uint32(TypeNodeInfoRequest) }
func (r *NodeInfoResponse) Type() uint32        { return uint32(TypeNodeInfoResponse) }
func (r *FindServiceRequest) Type() uint32      { return uint32(TypeFindServiceRequest) }
func (r *FindServiceResponse) Type() uint32     { return uint32(TypeFindServiceResponse) }
func (r *ConsensusStatusRequest) Type() uint32  { return uint32(TypeConsensusStatusRequest) }
func (r *ConsensusStatusResponse) Type() uint32 { return uint32(TypeConsensusStatusResponse) }
func (r *NetworkStatusRequest) Type() uint32    { return uint32(TypeNetworkStatusRequest) }
func (r *NetworkStatusResponse) Type() uint32   { return uint32(TypeNetworkStatusResponse) }
func (r *QueryRequest) Type() uint32           { return uint32(TypeQueryRequest) }
func (r *QueryResponse) Type() uint32          { return uint32(TypeQueryResponse) }

// CopyAsInterface implementations
func (r *NodeInfoRequest) CopyAsInterface() interface{} {
	if r == nil {
		return nil
	}
	return &NodeInfoRequest{Options: r.Options}
}

func (r *NodeInfoResponse) CopyAsInterface() interface{} {
	if r == nil {
		return nil
	}
	return &NodeInfoResponse{Info: r.Info}
}

func (r *FindServiceRequest) CopyAsInterface() interface{} {
	if r == nil {
		return nil
	}
	return &FindServiceRequest{Options: r.Options}
}

func (r *FindServiceResponse) CopyAsInterface() interface{} {
	if r == nil {
		return nil
	}
	return &FindServiceResponse{Services: r.Services}
}

func (r *ConsensusStatusRequest) CopyAsInterface() interface{} {
	if r == nil {
		return nil
	}
	return &ConsensusStatusRequest{Options: r.Options}
}

func (r *ConsensusStatusResponse) CopyAsInterface() interface{} {
	if r == nil {
		return nil
	}
	return &ConsensusStatusResponse{Status: r.Status}
}

func (r *NetworkStatusRequest) CopyAsInterface() interface{} {
	if r == nil {
		return nil
	}
	return &NetworkStatusRequest{Options: r.Options}
}

func (r *NetworkStatusResponse) CopyAsInterface() interface{} {
	if r == nil {
		return nil
	}
	return &NetworkStatusResponse{Status: r.Status}
}

func (r *QueryRequest) CopyAsInterface() interface{} {
	if r == nil {
		return nil
	}
	return &QueryRequest{Query: r.Query}
}

func (r *QueryResponse) CopyAsInterface() interface{} {
	if r == nil {
		return nil
	}
	return &QueryResponse{Result: r.Result}
}

// MarshalBinary implementations
func (r *NodeInfoRequest) MarshalBinary() ([]byte, error)         { return json.Marshal(r) }
func (r *NodeInfoResponse) MarshalBinary() ([]byte, error)        { return json.Marshal(r) }
func (r *FindServiceRequest) MarshalBinary() ([]byte, error)      { return json.Marshal(r) }
func (r *FindServiceResponse) MarshalBinary() ([]byte, error)     { return json.Marshal(r) }
func (r *ConsensusStatusRequest) MarshalBinary() ([]byte, error)  { return json.Marshal(r) }
func (r *ConsensusStatusResponse) MarshalBinary() ([]byte, error) { return json.Marshal(r) }
func (r *NetworkStatusRequest) MarshalBinary() ([]byte, error)    { return json.Marshal(r) }
func (r *NetworkStatusResponse) MarshalBinary() ([]byte, error)   { return json.Marshal(r) }
func (r *QueryRequest) MarshalBinary() ([]byte, error)           { return json.Marshal(r) }
func (r *QueryResponse) MarshalBinary() ([]byte, error)          { return json.Marshal(r) }

// UnmarshalBinary implementations
func (r *NodeInfoRequest) UnmarshalBinary(data []byte) error         { return json.Unmarshal(data, r) }
func (r *NodeInfoResponse) UnmarshalBinary(data []byte) error        { return json.Unmarshal(data, r) }
func (r *FindServiceRequest) UnmarshalBinary(data []byte) error      { return json.Unmarshal(data, r) }
func (r *FindServiceResponse) UnmarshalBinary(data []byte) error     { return json.Unmarshal(data, r) }
func (r *ConsensusStatusRequest) UnmarshalBinary(data []byte) error  { return json.Unmarshal(data, r) }
func (r *ConsensusStatusResponse) UnmarshalBinary(data []byte) error { return json.Unmarshal(data, r) }
func (r *NetworkStatusRequest) UnmarshalBinary(data []byte) error    { return json.Unmarshal(data, r) }
func (r *NetworkStatusResponse) UnmarshalBinary(data []byte) error   { return json.Unmarshal(data, r) }
func (r *QueryRequest) UnmarshalBinary(data []byte) error           { return json.Unmarshal(data, r) }
func (r *QueryResponse) UnmarshalBinary(data []byte) error          { return json.Unmarshal(data, r) }

// Service defines the interface for API services
type Service interface {
	NodeInfo(context.Context, *NodeInfoRequest) (*NodeInfoResponse, error)
	FindService(context.Context, *FindServiceRequest) (*FindServiceResponse, error)
	ConsensusStatus(context.Context, *ConsensusStatusRequest) (*ConsensusStatusResponse, error)
	NetworkStatus(context.Context, *NetworkStatusRequest) (*NetworkStatusResponse, error)
	Query(context.Context, *QueryRequest) (*QueryResponse, error)
}

// ServiceHandler implements the Service interface
type ServiceHandler struct {
	Service Service
}

func (s *ServiceHandler) NodeInfo(ctx context.Context, req *NodeInfoRequest) (*NodeInfoResponse, error) {
	return s.Service.NodeInfo(ctx, req)
}

func (s *ServiceHandler) FindService(ctx context.Context, req *FindServiceRequest) (*FindServiceResponse, error) {
	return s.Service.FindService(ctx, req)
}

func (s *ServiceHandler) ConsensusStatus(ctx context.Context, req *ConsensusStatusRequest) (*ConsensusStatusResponse, error) {
	return s.Service.ConsensusStatus(ctx, req)
}

func (s *ServiceHandler) NetworkStatus(ctx context.Context, req *NetworkStatusRequest) (*NetworkStatusResponse, error) {
	return s.Service.NetworkStatus(ctx, req)
}

func (s *ServiceHandler) Query(ctx context.Context, req *QueryRequest) (*QueryResponse, error) {
	return s.Service.Query(ctx, req)
}

// EventService implements a subset of the Service interface for event handling
type EventService struct{}

func (s *EventService) NodeInfo(ctx context.Context, req *NodeInfoRequest) (*NodeInfoResponse, error) {
	return nil, ErrMethodNotSupported
}

func (s *EventService) FindService(ctx context.Context, req *FindServiceRequest) (*FindServiceResponse, error) {
	return nil, ErrMethodNotSupported
}

func (s *EventService) ConsensusStatus(ctx context.Context, req *ConsensusStatusRequest) (*ConsensusStatusResponse, error) {
	return nil, ErrMethodNotSupported
}

func (s *EventService) NetworkStatus(ctx context.Context, req *NetworkStatusRequest) (*NetworkStatusResponse, error) {
	return nil, ErrMethodNotSupported
}

func (s *EventService) Query(ctx context.Context, req *QueryRequest) (*QueryResponse, error) {
	return nil, ErrMethodNotSupported
}

// ErrMethodNotSupported indicates that a method is not supported by the service
var ErrMethodNotSupported = &ErrorResponse{Code: "method_not_supported", Message: "Method not supported"}

// ServiceAdapter adapts a Service to handle messages.
type ServiceAdapter struct {
	Service Service
}

// Handle handles a message.
func (s *ServiceAdapter) Handle(ctx context.Context, msg Message) (Message, error) {
	switch msg := msg.(type) {
	case *NodeInfoRequest:
		info, err := s.Service.NodeInfo(ctx, msg)
		if err != nil {
			return nil, err
		}
		return &NodeInfoResponse{Info: info}, nil

	case *FindServiceRequest:
		results, err := s.Service.FindService(ctx, msg)
		if err != nil {
			return nil, err
		}
		return &FindServiceResponse{Services: results}, nil

	case *ConsensusStatusRequest:
		status, err := s.Service.ConsensusStatus(ctx, msg)
		if err != nil {
			return nil, err
		}
		return &ConsensusStatusResponse{Status: status}, nil

	case *NetworkStatusRequest:
		status, err := s.Service.NetworkStatus(ctx, msg)
		if err != nil {
			return nil, err
		}
		return &NetworkStatusResponse{Status: status}, nil

	case *QueryRequest:
		record, err := s.Service.Query(ctx, msg)
		if err != nil {
			return nil, err
		}
		return &QueryResponse{Result: record}, nil

	default:
		return nil, fmt.Errorf("unknown message type %T", msg)
	}
}

// Message interface implementations for request/response types
func (r *NodeInfoRequest) CopyAsInterface() Message {
	return &NodeInfoRequest{
		Options: r.Options,
	}
}

func (r *FindServiceRequest) CopyAsInterface() Message {
	return &FindServiceRequest{
		Options: r.Options,
	}
}

func (r *ConsensusStatusRequest) CopyAsInterface() Message {
	return &ConsensusStatusRequest{
		Options: r.Options,
	}
}

func (r *NetworkStatusRequest) CopyAsInterface() Message {
	return &NetworkStatusRequest{
		Options: r.Options,
	}
}

func (r *QueryRequest) CopyAsInterface() Message {
	return &QueryRequest{
		Query: r.Query,
	}
}

// Equal implementations for request/response types
func (r *NodeInfoRequest) Equal(other Message) bool {
	o, ok := other.(*NodeInfoRequest)
	if !ok {
		return false
	}
	return r.Options == o.Options
}

func (r *FindServiceRequest) Equal(other Message) bool {
	o, ok := other.(*FindServiceRequest)
	if !ok {
		return false
	}
	return r.Options == o.Options
}

func (r *ConsensusStatusRequest) Equal(other Message) bool {
	o, ok := other.(*ConsensusStatusRequest)
	if !ok {
		return false
	}
	return r.Options == o.Options
}

func (r *NetworkStatusRequest) Equal(other Message) bool {
	o, ok := other.(*NetworkStatusRequest)
	if !ok {
		return false
	}
	return r.Options == o.Options
}

func (r *QueryRequest) Equal(other Message) bool {
	o, ok := other.(*QueryRequest)
	if !ok {
		return false
	}
	return r.Query == o.Query
}
