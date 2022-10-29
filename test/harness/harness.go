package harness

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func New(tb testing.TB, services Services, stepper Stepper) *Harness {
	h := new(Harness)
	h.tb = tb
	h.services = services
	h.stepper = stepper
	return h
}

type Harness struct {
	tb       testing.TB
	services Services
	stepper  Stepper
}

type Services interface {
	api.Querier
	api.Submitter
}

type Stepper interface {
	Step() error
}

type TimeStep time.Duration

func (t TimeStep) Step() error {
	time.Sleep(time.Duration(t))
	return nil
}

func (h *Harness) Step() {
	h.tb.Helper()
	require.NoError(h.tb, h.stepper.Step())
}

func (h *Harness) StepN(n int) {
	h.tb.Helper()
	for i := 0; i < n; i++ {
		require.NoError(h.tb, h.stepper.Step())
	}
}

func (h *Harness) StepUntil(conditions ...Condition) {
	h.tb.Helper()
	for i := 0; ; i++ {
		if i >= 50 {
			h.tb.Fatal("Condition not met after 50 blocks")
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

type EnvelopeBuilder interface {
	Done() (*protocol.Envelope, error)
}

func (h *Harness) Submit(delivery *chain.Delivery) *protocol.TransactionStatus {
	h.tb.Helper()
	subs, err := h.services.Submit(context.Background(), &protocol.Envelope{Transaction: []*protocol.Transaction{delivery.Transaction}, Signatures: delivery.Signatures}, api.SubmitOptions{})
	require.NoError(h.tb, err)
	require.Len(h.tb, subs, 1)
	return subs[0].Status
}

func (h *Harness) SubmitSuccessfully(delivery *chain.Delivery) *protocol.TransactionStatus {
	h.tb.Helper()
	status := h.Submit(delivery)
	if status.Error != nil {
		require.NoError(h.tb, status.Error)
	}
	return status
}

func (h *Harness) BuildAndSubmit(b EnvelopeBuilder) *protocol.TransactionStatus {
	h.tb.Helper()
	env, err := b.Done()
	require.NoError(h.tb, err)
	require.Len(h.tb, env.Transaction, 1)
	subs, err := h.services.Submit(context.Background(), env, api.SubmitOptions{})
	require.NoError(h.tb, err)
	require.Len(h.tb, subs, 1)
	return subs[0].Status
}

func (h *Harness) BuildAndSubmitSuccessfully(b EnvelopeBuilder) *protocol.TransactionStatus {
	h.tb.Helper()
	status := h.BuildAndSubmit(b)
	if status.Error != nil {
		require.NoError(h.tb, status.Error)
	}
	return status
}
