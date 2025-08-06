package crosschain

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// SyntheticRequest represents a synthetic transaction to be sent
type SyntheticRequest struct {
	Messages     []messaging.Message
	Destination  *url.URL
	Context      context.Context
	ResponseChan chan error
}
