package crosschain

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// SyntheticRequest represents a synthetic transaction to be sent
type SyntheticRequest struct {
	Messages     []messaging.Message
	Destination  *url.URL
	Context      context.Context
	ResponseChan chan error
	SequenceNum  uint64
	Signatures   []protocol.Signature // For recovered transactions
}

// AnchorRequest represents an anchor transaction request
type AnchorRequest struct {
	Anchor      messaging.Message
	Destination *url.URL
	SequenceNum uint64
}
