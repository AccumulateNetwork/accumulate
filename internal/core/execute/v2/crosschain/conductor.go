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

// CrossChainConductor handles async processing of cross-partition transactions
type CrossChainConductor struct {
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

// NewCrossChainConductor creates and starts the conductor
func NewCrossChainConductor(dispatcher execute.Dispatcher, logger logging.OptionalLogger) *CrossChainConductor {
	cc := &CrossChainConductor{
		dispatcher:    dispatcher,
		logger:        logging.OptionalLogger{L: logger.With("module", "crosschain-conductor")},
		syntheticChan: make(chan *SyntheticRequest, 100), // Buffered channel for async processing
		stopChan:      make(chan struct{}),
	}

	// Start async processor
	cc.wg.Add(1)
	go cc.processSynthetics()

	return cc
}

// ProcessInbound processes inbound cross-partition messages through the conductor
func (cc *CrossChainConductor) ProcessInbound(ctx context.Context, messages []messaging.Message) []messaging.Message {
	// Phase 1: Direct pass-through for all messages (zero behavior change)
	// Future phases can add conductor logic here
	
	// Count and log cross-partition messages
	var crossPartitionCount int
	for _, msg := range messages {
		if cc.isCrossPartitionMessage(msg) {
			crossPartitionCount++
		}
	}
	
	if crossPartitionCount > 0 {
		cc.logger.Debug("Processing inbound cross-partition messages", "count", crossPartitionCount, "total_messages", len(messages))
	}
	
	// For now, return all messages unchanged
	return messages
}

// isCrossPartitionMessage determines if a message is a cross-partition anchor or synthetic transaction
func (cc *CrossChainConductor) isCrossPartitionMessage(msg messaging.Message) bool {
	switch msg.Type() {
	case messaging.MessageTypeSynthetic, messaging.MessageTypeBadSynthetic:
		return true
	case messaging.MessageTypeBlockAnchor:
		return true
	default:
		return false
	}
}

// SubmitSynthetic submits synthetic transactions for async processing
func (cc *CrossChainConductor) SubmitSynthetic(ctx context.Context, messages []messaging.Message, destination *url.URL) error {
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
		return errors.InternalError.With("conductor stopped")
	}
}

// processSynthetics is the main async processing loop
func (cc *CrossChainConductor) processSynthetics() {
	defer cc.wg.Done()
	cc.logger.Info("CrossChainConductor started")

	for {
		select {
		case req := <-cc.syntheticChan:
			cc.processSyntheticRequest(req)

		case <-cc.stopChan:
			cc.logger.Info("CrossChainConductor stopping")
			// Drain remaining requests
			for {
				select {
				case req := <-cc.syntheticChan:
					req.ResponseChan <- errors.InternalError.With("conductor stopping")
				default:
					return
				}
			}
		}
	}
}

// processSyntheticRequest processes a single synthetic transaction request
func (cc *CrossChainConductor) processSyntheticRequest(req *SyntheticRequest) {
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

// Stop gracefully stops the conductor
func (cc *CrossChainConductor) Stop() {
	close(cc.stopChan)
	cc.wg.Wait()
	cc.logger.Info("CrossChainConductor stopped")
}

// GetMetrics returns current processing metrics
func (cc *CrossChainConductor) GetMetrics() (sent, errors int64) {
	return atomic.LoadInt64(&cc.syntheticsSent), atomic.LoadInt64(&cc.syntheticsErrors)
}
