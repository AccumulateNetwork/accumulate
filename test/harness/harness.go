package harness

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/chain"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// New returns a new Harness with the given services and stepper.
func New(tb testing.TB, services Services, stepper Stepper) *Harness {
	h := new(Harness)
	h.TB = tb
	h.services = services
	h.stepper = stepper
	return h
}

// Harness is a test harness.
type Harness struct {
	TB       testing.TB
	services Services
	stepper  Stepper
}

// Services defines the services required by the harness.
type Services interface {
	api.Querier
	api.Submitter
}

// Stepper steps the simulation or waits for a block to complete.
type Stepper interface {
	Step() error
}

// TimeStep implements Stepper by sleeping.
type TimeStep time.Duration

// Step waits for some duration.
func (t TimeStep) Step() error {
	time.Sleep(time.Duration(t))
	return nil
}

// Step calls the stepper once.
func (h *Harness) Step() {
	h.TB.Helper()
	require.NoError(h.TB, h.stepper.Step())
}

// StepN calls the stepper N times.
func (h *Harness) StepN(n int) {
	h.TB.Helper()
	for i := 0; i < n; i++ {
		require.NoError(h.TB, h.stepper.Step())
	}
}

// StepUntil calls the stepper until all conditions are met. StepUntil fails if
// the conditions are not met within 50 steps.
func (h *Harness) StepUntil(conditions ...Condition) {
	h.TB.Helper()
	for i := 0; ; i++ {
		if i >= 50 {
			h.TB.Fatal("Condition not met after 50 blocks")
		}
		ok := true
		for _, c := range conditions {
			if !c(h) {
				ok = false
			}
		}
		if ok {
			break
		}
		h.Step()
	}
}

// Submit submits a delivery and returns the status.
func (h *Harness) Submit(delivery *chain.Delivery) *protocol.TransactionStatus {
	h.TB.Helper()
	subs, err := h.services.Submit(context.Background(), &protocol.Envelope{Transaction: []*protocol.Transaction{delivery.Transaction}, Signatures: delivery.Signatures}, api.SubmitOptions{})
	require.NoError(h.TB, err)
	require.Len(h.TB, subs, 1)
	return subs[0].Status
}

// SubmitSuccessfully submits a delivery and returns the status.
// SubmitSuccessfully fails if the transaction failed.
func (h *Harness) SubmitSuccessfully(delivery *chain.Delivery) *protocol.TransactionStatus {
	h.TB.Helper()
	status := h.Submit(delivery)
	if status.Error != nil {
		require.NoError(h.TB, status.Error)
	}
	return status
}

// EnvelopeBuilder builds an envelope.
type EnvelopeBuilder interface {
	Done() (*protocol.Envelope, error)
}

// BuildAndSubmit calls the envelope builder and submits the envelope.
func (h *Harness) BuildAndSubmit(b EnvelopeBuilder) *protocol.TransactionStatus {
	h.TB.Helper()
	env, err := b.Done()
	require.NoError(h.TB, err)
	require.Len(h.TB, env.Transaction, 1)
	subs, err := h.services.Submit(context.Background(), env, api.SubmitOptions{})
	require.NoError(h.TB, err)
	require.Len(h.TB, subs, 1)
	return subs[0].Status
}

// BuildAndSubmitSuccessfully calls the envelope builder and submits the
// envelope. BuildAndSubmitSuccessfully fails if the transaction failed.
func (h *Harness) BuildAndSubmitSuccessfully(b EnvelopeBuilder) *protocol.TransactionStatus {
	h.TB.Helper()
	status := h.BuildAndSubmit(b)
	if status.Error != nil {
		require.NoError(h.TB, status.Error)
	}
	return status
}
