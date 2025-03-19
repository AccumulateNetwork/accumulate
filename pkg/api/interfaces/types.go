// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package interfaces

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// Message represents a generic message in the system
type Message interface {
	Type() uint32
}

// MessageResponse represents a generic message response
type MessageResponse interface {
	Message
	IsResponse() bool
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error error
}

// ServiceAddress represents a service address
type ServiceAddress struct {
	Network string
	Type    uint32
}

// Transport defines the interface for message transport
type Transport interface {
	// RoundTrip sends a message and waits for a response
	RoundTrip(msg Message) (MessageResponse, error)
}

// Router defines the interface for message routing
type Router interface {
	// Route determines where a message should be sent
	Route(msg Message) (*url.URL, error)
}
