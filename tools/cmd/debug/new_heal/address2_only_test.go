// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

import (
	"fmt"
	"testing"
)

// TestConstructPartitionURL_Only tests the constructPartitionURL function in address2.go
func TestConstructPartitionURL_Only(t *testing.T) {
	tests := []struct {
		name        string
		partitionID string
		want        string
	}{
		{
			name:        "Directory Network",
			partitionID: "dn",
			want:        "acc://dn.acme",
		},
		{
			name:        "BVN Apollo",
			partitionID: "bvn-Apollo",
			want:        "acc://bvn-Apollo.acme",
		},
		{
			name:        "BVN Artemis",
			partitionID: "bvn-Artemis",
			want:        "acc://bvn-Artemis.acme",
		},
		{
			name:        "Empty partition",
			partitionID: "",
			want:        "acc://.acme", // Edge case, should probably handle this better
		},
	}

	addrDir := NewAddressDir()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := addrDir.constructPartitionURL(tt.partitionID)
			if got != tt.want {
				t.Errorf("constructPartitionURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestConstructAnchorURL_Only tests the constructAnchorURL function in address2.go
func TestConstructAnchorURL_Only(t *testing.T) {
	anchorAddrDir := NewAddressDir()
	
	tests := []struct {
		name        string
		partitionID string
		want        string
	}{
		{
			name:        "Directory Network",
			partitionID: "dn",
			want:        fmt.Sprintf("acc://dn.%s/anchors/dn", anchorAddrDir.GetNetworkName()),
		},
		{
			name:        "BVN Apollo",
			partitionID: "bvn-Apollo",
			want:        fmt.Sprintf("acc://dn.%s/anchors/bvn-Apollo", anchorAddrDir.GetNetworkName()),
		},
		{
			name:        "BVN Artemis",
			partitionID: "bvn-Artemis",
			want:        fmt.Sprintf("acc://dn.%s/anchors/bvn-Artemis", anchorAddrDir.GetNetworkName()),
		},
		{
			name:        "Empty partition",
			partitionID: "",
			want:        fmt.Sprintf("acc://dn.%s/anchors/", anchorAddrDir.GetNetworkName()), // Edge case, should probably handle this better
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := anchorAddrDir.constructAnchorURL(tt.partitionID)
			if got != tt.want {
				t.Errorf("constructAnchorURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestParseMultiaddress_Only tests the parseMultiaddress function in address2.go
func TestParseMultiaddress_Only(t *testing.T) {
	tests := []struct {
		name      string
		address   string
		wantIP    string
		wantPort  string
		wantPeerID string
		wantErr   bool
	}{
		{
			name:      "Valid multiaddress with IPv4",
			address:   "/ip4/65.108.73.121/tcp/16593/p2p/QmHash123",
			wantIP:    "65.108.73.121",
			wantPort:  "16593",
			wantPeerID: "QmHash123",
			wantErr:   false,
		},
		{
			name:      "Valid multiaddress with IPv6",
			address:   "/ip6/2001:db8::1/tcp/16593/p2p/QmHash456",
			wantIP:    "2001:db8::1",
			wantPort:  "16593",
			wantPeerID: "QmHash456",
			wantErr:   false,
		},
		{
			name:      "Invalid multiaddress",
			address:   "not-a-multiaddress",
			wantIP:    "",
			wantPort:  "",
			wantPeerID: "",
			wantErr:   true,
		},
		{
			name:      "Multiaddress without IP",
			address:   "/tcp/16593/p2p/QmHash789",
			wantIP:    "",
			wantPort:  "",
			wantPeerID: "",
			wantErr:   true,
		},
	}

	addrDir := NewAddressDir()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIP, gotPort, gotPeerID, err := addrDir.parseMultiaddress(tt.address)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseMultiaddress() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if gotIP != tt.wantIP {
					t.Errorf("parseMultiaddress() gotIP = %v, want %v", gotIP, tt.wantIP)
				}
				if gotPort != tt.wantPort {
					t.Errorf("parseMultiaddress() gotPort = %v, want %v", gotPort, tt.wantPort)
				}
				if gotPeerID != tt.wantPeerID {
					t.Errorf("parseMultiaddress() gotPeerID = %v, want %v", gotPeerID, tt.wantPeerID)
				}
			}
		})
	}
}

// TestValidateMultiaddress_Only tests the ValidateMultiaddress function in address2.go
func TestValidateMultiaddress_Only(t *testing.T) {
	tests := []struct {
		name      string
		address   string
		wantValid bool
	}{
		{
			name:      "Valid multiaddress",
			address:   "/ip4/65.108.73.121/tcp/16593/p2p/QmHash123",
			wantValid: true,
		},
		{
			name:      "Invalid multiaddress",
			address:   "not-a-multiaddress",
			wantValid: false,
		},
		{
			name:      "Multiaddress without IP",
			address:   "/tcp/16593/p2p/QmHash789",
			wantValid: false,
		},
	}

	addrDir := NewAddressDir()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, valid := addrDir.ValidateMultiaddress(tt.address)
			if valid != tt.wantValid {
				t.Errorf("ValidateMultiaddress() valid = %v, want %v", valid, tt.wantValid)
			}
		})
	}
}
