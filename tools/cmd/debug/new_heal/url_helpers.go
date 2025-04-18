// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

// SetURLHelper sets a URL helper value
func (a *AddressDir) SetURLHelper(key string, value string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	// Initialize the map if it doesn't exist
	if a.URLHelpers == nil {
		a.URLHelpers = make(map[string]string)
	}
	
	// Set the value
	a.URLHelpers[key] = value
	
	// Log the change
	if a.Logger != nil {
		a.Logger.Printf("Set URL helper %s = %s", key, value)
	}
}

// GetURLHelper gets a URL helper value
func (a *AddressDir) GetURLHelper(key string) (string, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	// Check if the map exists
	if a.URLHelpers == nil {
		return "", false
	}
	
	// Get the value
	value, exists := a.URLHelpers[key]
	return value, exists
}
