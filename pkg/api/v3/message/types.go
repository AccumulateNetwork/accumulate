// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package message

import (
	"io"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/generated"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

// Message is the interface that all messages must implement
type Message interface {
	encoding.UnionValue
	UnmarshalBinaryFrom(io.Reader) error
	CopyAsInterface() interface{}
}

// Type represents a message type
type Type uint32

// Message types
const (
	TypeNodeInfo Type = iota + 1
	TypeFindService
	TypeConsensusStatus
	TypeNetworkStatus
	TypeQuery
	TypeSubmit
	TypeFaucet
	TypeError
)

// NodeInfoRequest represents a request for node information
type NodeInfoRequest struct {
	Options *generated.NodeInfoOptions
}

func (r *NodeInfoRequest) Type() Type                                { return TypeNodeInfo }
func (r *NodeInfoRequest) MarshalBinary() ([]byte, error)            { return encoding.Marshal(r) }
func (r *NodeInfoRequest) UnmarshalBinary(data []byte) error         { return encoding.Unmarshal(data, r) }
func (r *NodeInfoRequest) UnmarshalBinaryFrom(rd io.Reader) error {
	data, err := io.ReadAll(rd)
	if err != nil {
		return err
	}
	return r.UnmarshalBinary(data)
}
func (r *NodeInfoRequest) CopyAsInterface() interface{} {
	if r == nil {
		return nil
	}
	return &NodeInfoRequest{Options: r.Options}
}

// NodeInfoResponse represents a response containing node information
type NodeInfoResponse struct {
	Info *generated.NodeInfo
}

func (r *NodeInfoResponse) Type() Type                                { return TypeNodeInfo }
func (r *NodeInfoResponse) MarshalBinary() ([]byte, error)            { return encoding.Marshal(r) }
func (r *NodeInfoResponse) UnmarshalBinary(data []byte) error         { return encoding.Unmarshal(data, r) }
func (r *NodeInfoResponse) UnmarshalBinaryFrom(rd io.Reader) error {
	data, err := io.ReadAll(rd)
	if err != nil {
		return err
	}
	return r.UnmarshalBinary(data)
}
func (r *NodeInfoResponse) CopyAsInterface() interface{} {
	if r == nil {
		return nil
	}
	return &NodeInfoResponse{Info: r.Info}
}

// FindServiceRequest represents a request to find a service
type FindServiceRequest struct {
	Options *generated.FindServiceOptions
}

func (r *FindServiceRequest) Type() Type                                { return TypeFindService }
func (r *FindServiceRequest) MarshalBinary() ([]byte, error)            { return encoding.Marshal(r) }
func (r *FindServiceRequest) UnmarshalBinary(data []byte) error         { return encoding.Unmarshal(data, r) }
func (r *FindServiceRequest) UnmarshalBinaryFrom(rd io.Reader) error {
	data, err := io.ReadAll(rd)
	if err != nil {
		return err
	}
	return r.UnmarshalBinary(data)
}
func (r *FindServiceRequest) CopyAsInterface() interface{} {
	if r == nil {
		return nil
	}
	return &FindServiceRequest{Options: r.Options}
}

// FindServiceResponse represents a response containing service information
type FindServiceResponse struct {
	Services []*generated.ServiceAddress
}

func (r *FindServiceResponse) Type() Type                                { return TypeFindService }
func (r *FindServiceResponse) MarshalBinary() ([]byte, error)            { return encoding.Marshal(r) }
func (r *FindServiceResponse) UnmarshalBinary(data []byte) error         { return encoding.Unmarshal(data, r) }
func (r *FindServiceResponse) UnmarshalBinaryFrom(rd io.Reader) error {
	data, err := io.ReadAll(rd)
	if err != nil {
		return err
	}
	return r.UnmarshalBinary(data)
}
func (r *FindServiceResponse) CopyAsInterface() interface{} {
	if r == nil {
		return nil
	}
	return &FindServiceResponse{Services: r.Services}
}

// ConsensusStatusRequest represents a request for consensus status
type ConsensusStatusRequest struct {
	Options *generated.ConsensusStatusOptions
}

func (r *ConsensusStatusRequest) Type() Type                                { return TypeConsensusStatus }
func (r *ConsensusStatusRequest) MarshalBinary() ([]byte, error)            { return encoding.Marshal(r) }
func (r *ConsensusStatusRequest) UnmarshalBinary(data []byte) error         { return encoding.Unmarshal(data, r) }
func (r *ConsensusStatusRequest) UnmarshalBinaryFrom(rd io.Reader) error {
	data, err := io.ReadAll(rd)
	if err != nil {
		return err
	}
	return r.UnmarshalBinary(data)
}
func (r *ConsensusStatusRequest) CopyAsInterface() interface{} {
	if r == nil {
		return nil
	}
	return &ConsensusStatusRequest{Options: r.Options}
}

// ConsensusStatusResponse represents a response containing consensus status
type ConsensusStatusResponse struct {
	Status *generated.ConsensusStatus
}

func (r *ConsensusStatusResponse) Type() Type                                { return TypeConsensusStatus }
func (r *ConsensusStatusResponse) MarshalBinary() ([]byte, error)            { return encoding.Marshal(r) }
func (r *ConsensusStatusResponse) UnmarshalBinary(data []byte) error         { return encoding.Unmarshal(data, r) }
func (r *ConsensusStatusResponse) UnmarshalBinaryFrom(rd io.Reader) error {
	data, err := io.ReadAll(rd)
	if err != nil {
		return err
	}
	return r.UnmarshalBinary(data)
}
func (r *ConsensusStatusResponse) CopyAsInterface() interface{} {
	if r == nil {
		return nil
	}
	return &ConsensusStatusResponse{Status: r.Status}
}

// NetworkStatusRequest represents a request for network status
type NetworkStatusRequest struct {
	Options *generated.NetworkStatusOptions
}

func (r *NetworkStatusRequest) Type() Type                                { return TypeNetworkStatus }
func (r *NetworkStatusRequest) MarshalBinary() ([]byte, error)            { return encoding.Marshal(r) }
func (r *NetworkStatusRequest) UnmarshalBinary(data []byte) error         { return encoding.Unmarshal(data, r) }
func (r *NetworkStatusRequest) UnmarshalBinaryFrom(rd io.Reader) error {
	data, err := io.ReadAll(rd)
	if err != nil {
		return err
	}
	return r.UnmarshalBinary(data)
}
func (r *NetworkStatusRequest) CopyAsInterface() interface{} {
	if r == nil {
		return nil
	}
	return &NetworkStatusRequest{Options: r.Options}
}

// NetworkStatusResponse represents a response containing network status
type NetworkStatusResponse struct {
	Status *generated.NetworkStatus
}

func (r *NetworkStatusResponse) Type() Type                                { return TypeNetworkStatus }
func (r *NetworkStatusResponse) MarshalBinary() ([]byte, error)            { return encoding.Marshal(r) }
func (r *NetworkStatusResponse) UnmarshalBinary(data []byte) error         { return encoding.Unmarshal(data, r) }
func (r *NetworkStatusResponse) UnmarshalBinaryFrom(rd io.Reader) error {
	data, err := io.ReadAll(rd)
	if err != nil {
		return err
	}
	return r.UnmarshalBinary(data)
}
func (r *NetworkStatusResponse) CopyAsInterface() interface{} {
	if r == nil {
		return nil
	}
	return &NetworkStatusResponse{Status: r.Status}
}

// QueryRequest represents a query request
type QueryRequest struct {
	Query *generated.QueryRequest
}

func (r *QueryRequest) Type() Type                                { return TypeQuery }
func (r *QueryRequest) MarshalBinary() ([]byte, error)            { return encoding.Marshal(r) }
func (r *QueryRequest) UnmarshalBinary(data []byte) error         { return encoding.Unmarshal(data, r) }
func (r *QueryRequest) UnmarshalBinaryFrom(rd io.Reader) error {
	data, err := io.ReadAll(rd)
	if err != nil {
		return err
	}
	return r.UnmarshalBinary(data)
}
func (r *QueryRequest) CopyAsInterface() interface{} {
	if r == nil {
		return nil
	}
	return &QueryRequest{Query: r.Query}
}

// RecordResponse represents a response containing records
type RecordResponse struct {
	Records []*generated.Record
}

func (r *RecordResponse) Type() Type                                { return TypeQuery }
func (r *RecordResponse) MarshalBinary() ([]byte, error)            { return encoding.Marshal(r) }
func (r *RecordResponse) UnmarshalBinary(data []byte) error         { return encoding.Unmarshal(data, r) }
func (r *RecordResponse) UnmarshalBinaryFrom(rd io.Reader) error {
	data, err := io.ReadAll(rd)
	if err != nil {
		return err
	}
	return r.UnmarshalBinary(data)
}
func (r *RecordResponse) CopyAsInterface() interface{} {
	if r == nil {
		return nil
	}
	return &RecordResponse{Records: r.Records}
}

// SubmitRequest represents a submit request
type SubmitRequest struct {
	Transaction []byte
	Options     *generated.SubmitOptions
}

func (r *SubmitRequest) Type() Type                                { return TypeSubmit }
func (r *SubmitRequest) MarshalBinary() ([]byte, error)            { return encoding.Marshal(r) }
func (r *SubmitRequest) UnmarshalBinary(data []byte) error         { return encoding.Unmarshal(data, r) }
func (r *SubmitRequest) UnmarshalBinaryFrom(rd io.Reader) error {
	data, err := io.ReadAll(rd)
	if err != nil {
		return err
	}
	return r.UnmarshalBinary(data)
}
func (r *SubmitRequest) CopyAsInterface() interface{} {
	if r == nil {
		return nil
	}
	return &SubmitRequest{
		Transaction: append([]byte(nil), r.Transaction...),
		Options:     r.Options,
	}
}

// SubmitResponse represents a submit response
type SubmitResponse struct {
	Results []*generated.SubmitResult
}

func (r *SubmitResponse) Type() Type                                { return TypeSubmit }
func (r *SubmitResponse) MarshalBinary() ([]byte, error)            { return encoding.Marshal(r) }
func (r *SubmitResponse) UnmarshalBinary(data []byte) error         { return encoding.Unmarshal(data, r) }
func (r *SubmitResponse) UnmarshalBinaryFrom(rd io.Reader) error {
	data, err := io.ReadAll(rd)
	if err != nil {
		return err
	}
	return r.UnmarshalBinary(data)
}
func (r *SubmitResponse) CopyAsInterface() interface{} {
	if r == nil {
		return nil
	}
	return &SubmitResponse{Results: r.Results}
}

// FaucetRequest represents a faucet request
type FaucetRequest struct {
	Account *url.URL
	Options *generated.FaucetOptions
}

func (r *FaucetRequest) Type() Type                                { return TypeFaucet }
func (r *FaucetRequest) MarshalBinary() ([]byte, error)            { return encoding.Marshal(r) }
func (r *FaucetRequest) UnmarshalBinary(data []byte) error         { return encoding.Unmarshal(data, r) }
func (r *FaucetRequest) UnmarshalBinaryFrom(rd io.Reader) error {
	data, err := io.ReadAll(rd)
	if err != nil {
		return err
	}
	return r.UnmarshalBinary(data)
}
func (r *FaucetRequest) CopyAsInterface() interface{} {
	if r == nil {
		return nil
	}
	return &FaucetRequest{
		Account: r.Account.Copy(),
		Options: r.Options,
	}
}

// FaucetResponse represents a faucet response
type FaucetResponse struct {
	Result *generated.SubmitResult
}

func (r *FaucetResponse) Type() Type                                { return TypeFaucet }
func (r *FaucetResponse) MarshalBinary() ([]byte, error)            { return encoding.Marshal(r) }
func (r *FaucetResponse) UnmarshalBinary(data []byte) error         { return encoding.Unmarshal(data, r) }
func (r *FaucetResponse) UnmarshalBinaryFrom(rd io.Reader) error {
	data, err := io.ReadAll(rd)
	if err != nil {
		return err
	}
	return r.UnmarshalBinary(data)
}
func (r *FaucetResponse) CopyAsInterface() interface{} {
	if r == nil {
		return nil
	}
	return &FaucetResponse{Result: r.Result}
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error string
}

func (r *ErrorResponse) Type() Type                                { return TypeError }
func (r *ErrorResponse) MarshalBinary() ([]byte, error)            { return encoding.Marshal(r) }
func (r *ErrorResponse) UnmarshalBinary(data []byte) error         { return encoding.Unmarshal(data, r) }
func (r *ErrorResponse) UnmarshalBinaryFrom(rd io.Reader) error {
	data, err := io.ReadAll(rd)
	if err != nil {
		return err
	}
	return r.UnmarshalBinary(data)
}
func (r *ErrorResponse) CopyAsInterface() interface{} {
	if r == nil {
		return nil
	}
	return &ErrorResponse{Error: r.Error}
}
func (r *ErrorResponse) Error() string {
	if r == nil {
		return "unknown error"
	}
	return r.Error
}
