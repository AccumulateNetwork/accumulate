package crosschain

import (
	"context"
	"sync"
	"sync/atomic"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// CrosschainCoordinator handles async processing of cross-partition transactions
type CrosschainCoordinator struct {
	// Infrastructure
	dispatcher execute.Dispatcher
	logger     logging.OptionalLogger

	// Async processing
	syntheticChan chan *SyntheticRequest
	stopChan      chan struct{}
	wg            sync.WaitGroup

	// Metrics (simple counters for Phase 1)
	syntheticsSent   int64
	syntheticsErrors int64
}

// NewCrosschainCoordinator creates and starts the coordinator
func NewCrosschainCoordinator(dispatcher execute.Dispatcher, logger logging.OptionalLogger) *CrosschainCoordinator {
	cc := &CrosschainCoordinator{
		dispatcher:    dispatcher,
		logger:        logging.OptionalLogger{L: logger.With("module", "crosschain-coordinator")},
		syntheticChan: make(chan *SyntheticRequest, 100), // Buffered channel for async processing
		stopChan:      make(chan struct{}),
	}

	// Start async processor
	cc.wg.Add(1)
	go cc.processSynthetics()

	return cc
}

// SubmitSynthetic submits synthetic transactions for async processing
func (cc *CrosschainCoordinator) SubmitSynthetic(ctx context.Context, messages []messaging.Message, destination *url.URL) error {
	responseChan := make(chan error, 1)
	req := &SyntheticRequest{
		Messages:     messages,
		Destination:  destination,
		Context:      ctx,
		ResponseChan: responseChan,
	}

	select {
	case cc.syntheticChan <- req:
		// Wait for async processing to complete
		return <-responseChan
	case <-ctx.Done():
		return ctx.Err()
	case <-cc.stopChan:
		return errors.InternalError.With("coordinator stopped")
	}
}

// processSynthetics is the main async processing loop
func (cc *CrosschainCoordinator) processSynthetics() {
	defer cc.wg.Done()
	cc.logger.Info("CrosschainCoordinator started")

	for {
		select {
		case req := <-cc.syntheticChan:
			cc.processSyntheticRequest(req)

		case <-cc.stopChan:
			cc.logger.Info("CrosschainCoordinator stopping")
			// Drain remaining requests
			for {
				select {
				case req := <-cc.syntheticChan:
					req.ResponseChan <- errors.InternalError.With("coordinator stopping")
				default:
					return
				}
			}
		}
	}
}

// processSyntheticRequest processes a single synthetic transaction request
func (cc *CrosschainCoordinator) processSyntheticRequest(req *SyntheticRequest) {
	// Phase 1: Direct pass-through to existing dispatcher (zero behavior change)
	env := &messaging.Envelope{Messages: req.Messages}
	err := cc.dispatcher.Submit(req.Context, req.Destination, env)

	// Update metrics
	if err != nil {
		atomic.AddInt64(&cc.syntheticsErrors, 1)
		cc.logger.Error("Synthetic transaction failed", "destination", req.Destination, "error", err)
	} else {
		atomic.AddInt64(&cc.syntheticsSent, 1)
		cc.logger.Debug("Synthetic transaction sent", "destination", req.Destination, "messages", len(req.Messages))
	}

	// Send response back to caller
	req.ResponseChan <- err
}

// Stop gracefully stops the coordinator
func (cc *CrosschainCoordinator) Stop() {
	close(cc.stopChan)
	cc.wg.Wait()
	cc.logger.Info("CrosschainCoordinator stopped")
}

// GetMetrics returns current processing metrics
func (cc *CrosschainCoordinator) GetMetrics() (sent, errors int64) {
	return atomic.LoadInt64(&cc.syntheticsSent), atomic.LoadInt64(&cc.syntheticsErrors)
}
