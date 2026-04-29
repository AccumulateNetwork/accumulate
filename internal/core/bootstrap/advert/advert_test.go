// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package advert

import (
	"sync"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
)

// fakeRegistrar records every RegisterService call.
type fakeRegistrar struct {
	mu       sync.Mutex
	registered []*api.ServiceAddress
}

func (f *fakeRegistrar) RegisterService(sa *api.ServiceAddress, _ func(message.Stream)) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.registered = append(f.registered, sa)
	return true
}

func (f *fakeRegistrar) lastArg() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.registered) == 0 {
		return ""
	}
	return f.registered[len(f.registered)-1].Argument
}

func TestServiceAddress(t *testing.T) {
	tests := []struct {
		partition string
		state     nodestate.State
		wantArg   string
		wantNil   bool
	}{
		{"Directory", nodestate.StateActive, "directory:active", false},
		{"Directory", nodestate.StateComplete, "directory:complete", false},
		{"Apollo", nodestate.StateActive, "apollo:active", false},
		{"Apollo", nodestate.StateBooting, "", true},
		{"Apollo", nodestate.StateUnknown, "", true},
	}
	for _, tc := range tests {
		got := ServiceAddress(tc.partition, tc.state)
		if tc.wantNil {
			if got != nil {
				t.Errorf("partition=%s state=%v: want nil, got %+v", tc.partition, tc.state, got)
			}
			continue
		}
		if got == nil {
			t.Errorf("partition=%s state=%v: got nil", tc.partition, tc.state)
			continue
		}
		if got.Type != api.ServiceTypeBootstrap {
			t.Errorf("Type=%v, want Bootstrap", got.Type)
		}
		if got.Argument != tc.wantArg {
			t.Errorf("Argument=%q, want %q", got.Argument, tc.wantArg)
		}
	}
}

// TestPublisher_PublishesOnPromotion — Wire to a fresh BOOTING
// machine, promote to ACTIVE, expect the active service to register.
func TestPublisher_PublishesOnPromotion(t *testing.T) {
	reg := &fakeRegistrar{}
	pub, err := New(reg, "Directory")
	if err != nil {
		t.Fatal(err)
	}
	m := nodestate.New()
	pub.Wire(m)

	// BOOTING: no registration.
	if len(reg.registered) != 0 {
		t.Errorf("expected zero registrations while BOOTING, got %d", len(reg.registered))
	}

	// Promote to ACTIVE.
	if !m.PromoteToActive([32]byte{0xab}, 42) {
		t.Fatal("PromoteToActive returned false")
	}
	if reg.lastArg() != "directory:active" {
		t.Errorf("lastArg=%q, want directory:active", reg.lastArg())
	}
}

// TestPublisher_WireOnAlreadyActive — Wire after the machine is
// already past BOOTING. Should publish immediately.
func TestPublisher_WireOnAlreadyActive(t *testing.T) {
	reg := &fakeRegistrar{}
	pub, err := New(reg, "Apollo")
	if err != nil {
		t.Fatal(err)
	}
	m, err := nodestate.Restore(nodestate.StateActive, 100, [32]byte{0x01}, 0)
	if err != nil {
		t.Fatal(err)
	}
	pub.Wire(m)
	if reg.lastArg() != "apollo:active" {
		t.Errorf("lastArg=%q, want apollo:active", reg.lastArg())
	}
}

// TestNew_RejectsMissingInputs — guards.
func TestNew_RejectsMissingInputs(t *testing.T) {
	if _, err := New(nil, "X"); err == nil {
		t.Error("expected err for nil registrar")
	}
	if _, err := New(&fakeRegistrar{}, ""); err == nil {
		t.Error("expected err for empty partition")
	}
}
