// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

import (
	"testing"
)

func TestURLHelpers(t *testing.T) {
	// Create a new AddressDir
	addr := NewAddressDir()
	
	// Test setting and getting a URL helper
	addr.SetURLHelper("test", "value")
	value, exists := addr.GetURLHelper("test")
	if !exists {
		t.Errorf("Expected URL helper 'test' to exist")
	}
	if value != "value" {
		t.Errorf("Expected URL helper 'test' to have value 'value', got '%s'", value)
	}
	
	// Test getting a non-existent URL helper
	value, exists = addr.GetURLHelper("nonexistent")
	if exists {
		t.Errorf("Expected URL helper 'nonexistent' to not exist")
	}
	if value != "" {
		t.Errorf("Expected empty value for non-existent URL helper, got '%s'", value)
	}
	
	// Test overwriting a URL helper
	addr.SetURLHelper("test", "new-value")
	value, exists = addr.GetURLHelper("test")
	if !exists {
		t.Errorf("Expected URL helper 'test' to exist after overwriting")
	}
	if value != "new-value" {
		t.Errorf("Expected URL helper 'test' to have value 'new-value' after overwriting, got '%s'", value)
	}
}
