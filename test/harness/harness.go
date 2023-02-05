// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package harness

import (
	"context"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// New returns a new Harness with the given services and stepper.
func New(tb testing.TB, services Services, stepper Stepper) *Harness {
	color.NoColor = false
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
	api.NetworkService
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

// BlockStep implements [Stepper] by waiting for a block event.
type BlockStep <-chan api.Event

// Step processes events until there is an error or a block event.
func (s BlockStep) Step() error {
	for {
		switch event := (<-s).(type) {
		case *api.ErrorEvent:
			return event.Err
		case *api.BlockEvent:
			return nil
		}
	}
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
		h.Step()
	}
}

// StepUntil calls the stepper until all conditions are met. StepUntil fails if
// the conditions are not met within 50 steps.
func (h *Harness) StepUntil(conditions ...Condition) {
	h.TB.Helper()
	for i := 0; ; i++ {
		if i >= 50 {
			h.TB.Fatal(color.RedString("Condition not met after 50 blocks"))
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

// Submit submits an envelope and returns the status.
func (h *Harness) Submit(envelope *messaging.Envelope) *protocol.TransactionStatus {
	h.TB.Helper()
	subs, err := h.services.Submit(context.Background(), envelope, api.SubmitOptions{})
	require.NoError(h.TB, err)
	require.Len(h.TB, subs, 1)
	return subs[0].Status
}

// SubmitSuccessfully submits an envelope and returns the status.
// SubmitSuccessfully fails if the transaction failed.
func (h *Harness) SubmitSuccessfully(envelope *messaging.Envelope) *protocol.TransactionStatus {
	h.TB.Helper()
	status := h.Submit(envelope)
	if status.Error != nil {
		require.NoError(h.TB, status.Error)
	}
	return status
}

// EnvelopeBuilder builds an envelope.
type EnvelopeBuilder interface {
	Done() (*messaging.Envelope, error)
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
