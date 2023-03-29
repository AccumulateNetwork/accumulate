// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package message

// GENERATED BY go run ./tools/cmd/gen-enum. DO NOT EDIT.

import (
	"encoding/json"
	"fmt"
	"strings"
)

// TypeNodeInfoRequest .
const TypeNodeInfoRequest Type = 1

// TypeFindServiceRequest .
const TypeFindServiceRequest Type = 2

// TypeConsensusStatusRequest .
const TypeConsensusStatusRequest Type = 3

// TypeNetworkStatusRequest .
const TypeNetworkStatusRequest Type = 4

// TypeMetricsRequest .
const TypeMetricsRequest Type = 5

// TypeQueryRequest .
const TypeQueryRequest Type = 6

// TypeSubmitRequest .
const TypeSubmitRequest Type = 7

// TypeValidateRequest .
const TypeValidateRequest Type = 8

// TypeSubscribeRequest .
const TypeSubscribeRequest Type = 9

// TypeFaucetRequest .
const TypeFaucetRequest Type = 10

// TypeErrorResponse .
const TypeErrorResponse Type = 32

// TypeNodeInfoResponse .
const TypeNodeInfoResponse Type = 33

// TypeFindServiceResponse .
const TypeFindServiceResponse Type = 34

// TypeConsensusStatusResponse .
const TypeConsensusStatusResponse Type = 35

// TypeNetworkStatusResponse .
const TypeNetworkStatusResponse Type = 36

// TypeMetricsResponse .
const TypeMetricsResponse Type = 37

// TypeRecordResponse .
const TypeRecordResponse Type = 38

// TypeSubmitResponse .
const TypeSubmitResponse Type = 39

// TypeValidateResponse .
const TypeValidateResponse Type = 40

// TypeSubscribeResponse .
const TypeSubscribeResponse Type = 41

// TypeFaucetResponse .
const TypeFaucetResponse Type = 42

// TypeEvent .
const TypeEvent Type = 64

// TypePrivateSequenceRequest .
const TypePrivateSequenceRequest Type = 128

// TypePrivateSequenceResponse .
const TypePrivateSequenceResponse Type = 129

// TypeAddressed .
const TypeAddressed Type = 255

// GetEnumValue returns the value of the Type
func (v Type) GetEnumValue() uint64 { return uint64(v) }

// SetEnumValue sets the value. SetEnumValue returns false if the value is invalid.
func (v *Type) SetEnumValue(id uint64) bool {
	u := Type(id)
	switch u {
	case TypeNodeInfoRequest, TypeFindServiceRequest, TypeConsensusStatusRequest, TypeNetworkStatusRequest, TypeMetricsRequest, TypeQueryRequest, TypeSubmitRequest, TypeValidateRequest, TypeSubscribeRequest, TypeFaucetRequest, TypeErrorResponse, TypeNodeInfoResponse, TypeFindServiceResponse, TypeConsensusStatusResponse, TypeNetworkStatusResponse, TypeMetricsResponse, TypeRecordResponse, TypeSubmitResponse, TypeValidateResponse, TypeSubscribeResponse, TypeFaucetResponse, TypeEvent, TypePrivateSequenceRequest, TypePrivateSequenceResponse, TypeAddressed:
		*v = u
		return true
	}
	if u, ok := messageRegistry.TypeByValue(id); ok {
		*v = u
		return true
	}
	return false
}

// String returns the name of the Type.
func (v Type) String() string {
	switch v {
	case TypeNodeInfoRequest:
		return "nodeInfoRequest"
	case TypeFindServiceRequest:
		return "findServiceRequest"
	case TypeConsensusStatusRequest:
		return "consensusStatusRequest"
	case TypeNetworkStatusRequest:
		return "networkStatusRequest"
	case TypeMetricsRequest:
		return "metricsRequest"
	case TypeQueryRequest:
		return "queryRequest"
	case TypeSubmitRequest:
		return "submitRequest"
	case TypeValidateRequest:
		return "validateRequest"
	case TypeSubscribeRequest:
		return "subscribeRequest"
	case TypeFaucetRequest:
		return "faucetRequest"
	case TypeErrorResponse:
		return "errorResponse"
	case TypeNodeInfoResponse:
		return "nodeInfoResponse"
	case TypeFindServiceResponse:
		return "findServiceResponse"
	case TypeConsensusStatusResponse:
		return "consensusStatusResponse"
	case TypeNetworkStatusResponse:
		return "networkStatusResponse"
	case TypeMetricsResponse:
		return "metricsResponse"
	case TypeRecordResponse:
		return "recordResponse"
	case TypeSubmitResponse:
		return "submitResponse"
	case TypeValidateResponse:
		return "validateResponse"
	case TypeSubscribeResponse:
		return "subscribeResponse"
	case TypeFaucetResponse:
		return "faucetResponse"
	case TypeEvent:
		return "event"
	case TypePrivateSequenceRequest:
		return "privateSequenceRequest"
	case TypePrivateSequenceResponse:
		return "privateSequenceResponse"
	case TypeAddressed:
		return "addressed"
	}
	if s, ok := messageRegistry.TypeName(v); ok {
		return s
	}
	return fmt.Sprintf("Type:%d", v)
}

// TypeByName returns the named Type.
func TypeByName(name string) (Type, bool) {
	switch strings.ToLower(name) {
	case "nodeinforequest":
		return TypeNodeInfoRequest, true
	case "findservicerequest":
		return TypeFindServiceRequest, true
	case "consensusstatusrequest":
		return TypeConsensusStatusRequest, true
	case "networkstatusrequest":
		return TypeNetworkStatusRequest, true
	case "metricsrequest":
		return TypeMetricsRequest, true
	case "queryrequest":
		return TypeQueryRequest, true
	case "submitrequest":
		return TypeSubmitRequest, true
	case "validaterequest":
		return TypeValidateRequest, true
	case "subscriberequest":
		return TypeSubscribeRequest, true
	case "faucetrequest":
		return TypeFaucetRequest, true
	case "errorresponse":
		return TypeErrorResponse, true
	case "nodeinforesponse":
		return TypeNodeInfoResponse, true
	case "findserviceresponse":
		return TypeFindServiceResponse, true
	case "consensusstatusresponse":
		return TypeConsensusStatusResponse, true
	case "networkstatusresponse":
		return TypeNetworkStatusResponse, true
	case "metricsresponse":
		return TypeMetricsResponse, true
	case "recordresponse":
		return TypeRecordResponse, true
	case "submitresponse":
		return TypeSubmitResponse, true
	case "validateresponse":
		return TypeValidateResponse, true
	case "subscriberesponse":
		return TypeSubscribeResponse, true
	case "faucetresponse":
		return TypeFaucetResponse, true
	case "event":
		return TypeEvent, true
	case "privatesequencerequest":
		return TypePrivateSequenceRequest, true
	case "privatesequenceresponse":
		return TypePrivateSequenceResponse, true
	case "addressed":
		return TypeAddressed, true
	}
	if typ, ok := messageRegistry.TypeByName(name); ok {
		return typ, true
	}
	return 0, false
}

// MarshalJSON marshals the Type to JSON as a string.
func (v Type) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

// UnmarshalJSON unmarshals the Type from JSON as a string.
func (v *Type) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	var ok bool
	*v, ok = TypeByName(s)
	if !ok || strings.ContainsRune(v.String(), ':') {
		return fmt.Errorf("invalid Type %q", s)
	}
	return nil
}
