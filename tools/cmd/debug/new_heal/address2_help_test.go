// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

import (
	"testing"
)

func TestHelperConstructPartitionURL(t *testing.T) {
	// Create a test AddressDir with NetworkInfo
	dir := &AddressDir{
		NetworkInfo: &NetworkInfo{
			ID: "acme",
		},
	}

	// Test cases
	tests := []struct {
		name        string
		partitionID string
		want        string
	}{
		{
			name:        "BVN Apollo",
			partitionID: "bvn-Apollo",
			want:        "acc://bvn-Apollo.acme",
		},
		{
			name:        "DN",
			partitionID: "dn",
			want:        "acc://dn.acme",
		},
	}

	// Run tests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dir.constructPartitionURL(tt.partitionID)
			if got != tt.want {
				t.Errorf("constructPartitionURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHelperConstructAnchorURL(t *testing.T) {
	// Create a test AddressDir with NetworkInfo
	dir := &AddressDir{
		NetworkInfo: &NetworkInfo{
			ID: "acme",
		},
	}

	// Test cases
	tests := []struct {
		name        string
		partitionID string
		want        string
	}{
		{
			name:        "Apollo",
			partitionID: "Apollo",
			want:        "acc://dn.acme/anchors/Apollo",
		},
		{
			name:        "Chandrayaan",
			partitionID: "Chandrayaan",
			want:        "acc://dn.acme/anchors/Chandrayaan",
		},
	}

	// Run tests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dir.constructAnchorURL(tt.partitionID)
			if got != tt.want {
				t.Errorf("constructAnchorURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindValidatorByID(t *testing.T) {
	// Create test validators
	dnValidator := Validator{
		ID:     "dn-validator-1",
		PeerID: "dn-peer-1",
		Name:   "DN Validator 1",
	}

	bvnValidator := Validator{
		ID:     "bvn-validator-1",
		PeerID: "bvn-peer-1",
		Name:   "BVN Validator 1",
	}

	legacyValidator := &Validator{
		ID:     "legacy-validator",
		PeerID: "legacy-peer",
		Name:   "Legacy Validator",
	}

	// Create a test AddressDir with validators
	dir := &AddressDir{
		DNValidators:  []Validator{dnValidator},
		BVNValidators: [][]Validator{{bvnValidator}},
		Validators:    []*Validator{legacyValidator},
	}

	// Test cases
	tests := []struct {
		name   string
		peerID string
		want   string // Name of the validator we expect to find
	}{
		{
			name:   "Find DN validator by ID",
			peerID: "dn-validator-1",
			want:   "DN Validator 1",
		},
		{
			name:   "Find DN validator by PeerID",
			peerID: "dn-peer-1",
			want:   "DN Validator 1",
		},
		{
			name:   "Find BVN validator by ID",
			peerID: "bvn-validator-1",
			want:   "BVN Validator 1",
		},
		{
			name:   "Find BVN validator by PeerID",
			peerID: "bvn-peer-1",
			want:   "BVN Validator 1",
		},
		{
			name:   "Find legacy validator by ID",
			peerID: "legacy-validator",
			want:   "Legacy Validator",
		},
		{
			name:   "Find legacy validator by PeerID",
			peerID: "legacy-peer",
			want:   "Legacy Validator",
		},
		{
			name:   "Validator not found",
			peerID: "nonexistent",
			want:   "",
		},
	}

	// Run tests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dir.findValidatorByID(tt.peerID)
			if got == nil {
				if tt.want != "" {
					t.Errorf("findValidatorByID() = nil, want validator with name %v", tt.want)
				}
				return
			}
			if got.Name != tt.want {
				t.Errorf("findValidatorByID() = %v, want %v", got.Name, tt.want)
			}
		})
	}
}

func TestHelperValidateMultiaddress(t *testing.T) {
	dir := &AddressDir{}

	// Test cases
	tests := []struct {
		name    string
		address string
		wantIP  string
		wantOK  bool
	}{
		{
			name:    "Valid multiaddress",
			address: "/ip4/127.0.0.1/tcp/8080/p2p/QmPeerID",
			wantIP:  "127.0.0.1",
			wantOK:  true,
		},
		{
			name:    "Invalid multiaddress",
			address: "invalid",
			wantIP:  "",
			wantOK:  false,
		},
	}

	// Run tests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip, _, _, ok := dir.ValidateMultiaddress(tt.address)
			if ok != tt.wantOK {
				t.Errorf("ValidateMultiaddress() ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && ip != tt.wantIP {
				t.Errorf("ValidateMultiaddress() ip = %v, want %v", ip, tt.wantIP)
			}
		})
	}
}

func TestHelperParseMultiaddress(t *testing.T) {
	dir := &AddressDir{}

	// Test cases
	tests := []struct {
		name       string
		address    string
		wantIP     string
		wantPort   string
		wantPeerID string
		wantErr    bool
	}{
		{
			name:       "Valid multiaddress with all components",
			address:    "/ip4/127.0.0.1/tcp/8080/p2p/QmPeerID",
			wantIP:     "127.0.0.1",
			wantPort:   "8080",
			wantPeerID: "QmPeerID",
			wantErr:    false,
		},
		{
			name:       "Valid multiaddress without peer ID",
			address:    "/ip4/127.0.0.1/tcp/8080",
			wantIP:     "127.0.0.1",
			wantPort:   "8080",
			wantPeerID: "",
			wantErr:    false,
		},
		{
			name:       "Invalid multiaddress - manual parsing",
			address:    "ip4/127.0.0.1/tcp/8080/p2p/QmPeerID",
			wantIP:     "127.0.0.1",
			wantPort:   "8080",
			wantPeerID: "QmPeerID",
			wantErr:    false,
		},
		{
			name:       "Invalid multiaddress - no IP",
			address:    "/tcp/8080/p2p/QmPeerID",
			wantIP:     "",
			wantPort:   "",
			wantPeerID: "",
			wantErr:    true,
		},
	}

	// Run tests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip, port, peerID, err := dir.parseMultiaddress(tt.address)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseMultiaddress() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if ip != tt.wantIP {
					t.Errorf("parseMultiaddress() ip = %v, want %v", ip, tt.wantIP)
				}
				if port != tt.wantPort {
					t.Errorf("parseMultiaddress() port = %v, want %v", port, tt.wantPort)
				}
				if peerID != tt.wantPeerID {
					t.Errorf("parseMultiaddress() peerID = %v, want %v", peerID, tt.wantPeerID)
				}
			}
		})
	}
}

func TestGetPeerRPCEndpoint(t *testing.T) {
	// Create test validators
	validator := Validator{
		ID:         "validator-1",
		PeerID:     "peer-1",
		RPCAddress: "http://127.0.0.1:8080",
	}

	// Create a test AddressDir with validators
	dir := &AddressDir{
		DNValidators: []Validator{validator},
	}

	// Test cases
	tests := []struct {
		name string
		id   string
		want string
	}{
		{
			name: "Find validator by ID",
			id:   "validator-1",
			want: "http://127.0.0.1:8080",
		},
		{
			name: "Find validator by PeerID",
			id:   "peer-1",
			want: "http://127.0.0.1:8080",
		},
		{
			name: "Validator not found",
			id:   "nonexistent",
			want: "",
		},
	}

	// Run tests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dir.GetPeerRPCEndpoint(tt.id)
			if got != tt.want {
				t.Errorf("GetPeerRPCEndpoint() = %v, want %v", got, tt.want)
			}
		})
	}
}
