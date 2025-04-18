// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

import (
	"testing"
)

// TestAddValidator tests the AddValidator function
func TestAddValidator(t *testing.T) {
	// Create a new AddressDir instance
	addrDir := NewTestAddressDir()
	
	// Create a DN validator
	validator := TestValidator("peer1", "validator1", "dn", "dn")
	
	// Add the validator
	err := addrDir.AddValidator(validator)
	if err != nil {
		t.Fatalf("Failed to add validator: %v", err)
	}
	
	// Verify the validator was added correctly
	retrievedValidator := addrDir.GetValidator("peer1")
	if retrievedValidator == nil {
		t.Fatalf("Failed to retrieve validator after adding")
	}
	
	if retrievedValidator.PeerID != "peer1" {
		t.Errorf("Expected validator PeerID to be 'peer1', got '%s'", retrievedValidator.PeerID)
	}
	
	if retrievedValidator.Name != "validator1" {
		t.Errorf("Expected validator Name to be 'validator1', got '%s'", retrievedValidator.Name)
	}
}

// TestGetValidator tests the GetValidator function
func TestGetValidator(t *testing.T) {
	// Create a new AddressDir instance
	addrDir := NewTestAddressDir()
	
	// Create and add a DN validator
	validator1 := TestValidator("peer1", "validator1", "dn", "dn")
	err := addrDir.AddValidator(validator1)
	if err != nil {
		t.Fatalf("Failed to add validator1: %v", err)
	}
	
	// Create and add a BVN validator
	validator2 := TestValidator("peer2", "validator2", "bvn-Apollo", "bvn")
	err = addrDir.AddValidator(validator2)
	if err != nil {
		t.Fatalf("Failed to add validator2: %v", err)
	}
	
	// Test getting an existing DN validator
	retrievedValidator := addrDir.GetValidator("peer1")
	if retrievedValidator == nil {
		t.Fatalf("Failed to retrieve DN validator")
	}
	if retrievedValidator.PeerID != "peer1" {
		t.Errorf("Expected PeerID to be 'peer1', got '%s'", retrievedValidator.PeerID)
	}
	if retrievedValidator.PartitionType != "dn" {
		t.Errorf("Expected PartitionType to be 'dn', got '%s'", retrievedValidator.PartitionType)
	}
	
	// Test getting an existing BVN validator
	retrievedValidator = addrDir.GetValidator("peer2")
	if retrievedValidator == nil {
		t.Fatalf("Failed to retrieve BVN validator")
	}
	if retrievedValidator.PeerID != "peer2" {
		t.Errorf("Expected PeerID to be 'peer2', got '%s'", retrievedValidator.PeerID)
	}
	if retrievedValidator.PartitionType != "bvn" {
		t.Errorf("Expected PartitionType to be 'bvn', got '%s'", retrievedValidator.PartitionType)
	}
	
	// Test getting a non-existent validator
	retrievedValidator = addrDir.GetValidator("non-existent")
	if retrievedValidator != nil {
		t.Errorf("Expected nil for non-existent validator, got %+v", retrievedValidator)
	}
}

// TestUpdateValidator tests the UpdateValidator function
func TestUpdateValidator(t *testing.T) {
	// Create a new AddressDir instance
	addrDir := NewTestAddressDir()
	
	// Create and add a validator
	validator := TestValidator("peer1", "validator1", "dn", "dn")
	err := addrDir.AddValidator(validator)
	if err != nil {
		t.Fatalf("Failed to add validator: %v", err)
	}
	
	// Get the validator to update
	retrievedValidator := addrDir.GetValidator("peer1")
	if retrievedValidator == nil {
		t.Fatalf("Failed to retrieve validator")
	}
	
	// Modify the validator
	retrievedValidator.Name = "updated_name"
	retrievedValidator.Status = "inactive"
	
	// Update the validator
	success := addrDir.UpdateValidator(retrievedValidator)
	if !success {
		t.Errorf("UpdateValidator returned false, expected true")
	}
	
	// Verify the update
	updatedValidator := addrDir.GetValidator("peer1")
	if updatedValidator == nil {
		t.Fatalf("Failed to retrieve updated validator")
	}
	
	if updatedValidator.Name != "updated_name" {
		t.Errorf("Expected Name to be 'updated_name', got '%s'", updatedValidator.Name)
	}
	
	if updatedValidator.Status != "inactive" {
		t.Errorf("Expected Status to be 'inactive', got '%s'", updatedValidator.Status)
	}
	
	// Try to update a non-existent validator
	nonExistentValidator := &Validator{PeerID: "non-existent"}
	success = addrDir.UpdateValidator(nonExistentValidator)
	if success {
		t.Errorf("UpdateValidator returned true for non-existent validator, expected false")
	}
}

// TestValidatorProblematic tests the MarkValidatorProblematic and ClearValidatorProblematic functions
func TestValidatorProblematic(t *testing.T) {
	// Create a new AddressDir instance
	addrDir := NewTestAddressDir()
	
	// Create and add a validator
	validator := TestValidator("peer1", "validator1", "dn", "dn")
	err := addrDir.AddValidator(validator)
	if err != nil {
		t.Fatalf("Failed to add validator: %v", err)
	}
	
	// Mark the validator as problematic
	success := addrDir.MarkValidatorProblematic("peer1", "Test reason")
	if !success {
		t.Errorf("MarkValidatorProblematic returned false, expected true")
	}
	
	// Verify the validator is marked as problematic
	problematicValidator := addrDir.GetValidator("peer1")
	if problematicValidator == nil {
		t.Fatalf("Failed to retrieve problematic validator")
	}
	
	if !problematicValidator.IsProblematic {
		t.Errorf("Expected validator to be marked as problematic, but it wasn't")
	}
	
	if problematicValidator.ProblemReason != "Test reason" {
		t.Errorf("Expected problem reason to be 'Test reason', got '%s'", problematicValidator.ProblemReason)
	}
	
	// Clear the problematic status
	success = addrDir.ClearValidatorProblematic("peer1")
	if !success {
		t.Errorf("ClearValidatorProblematic returned false, expected true")
	}
	
	// Verify the problematic status was cleared
	clearedValidator := addrDir.GetValidator("peer1")
	if clearedValidator == nil {
		t.Fatalf("Failed to retrieve cleared validator")
	}
	
	if clearedValidator.IsProblematic {
		t.Errorf("Expected validator to not be problematic after clearing, but it was")
	}
	
	// Try to mark a non-existent validator as problematic
	success = addrDir.MarkValidatorProblematic("non-existent", "Test reason")
	if success {
		t.Errorf("MarkValidatorProblematic returned true for non-existent validator, expected false")
	}
	
	// Try to clear a non-existent validator's problematic status
	success = addrDir.ClearValidatorProblematic("non-existent")
	if success {
		t.Errorf("ClearValidatorProblematic returned true for non-existent validator, expected false")
	}
}

// TestRequestTypeAvoidance tests the MarkValidatorRequestTypeProblematic function
func TestRequestTypeAvoidance(t *testing.T) {
	// Create a new AddressDir instance
	addrDir := NewTestAddressDir()
	
	// Create and add a validator
	validator := TestValidator("peer1", "validator1", "dn", "dn")
	err := addrDir.AddValidator(validator)
	if err != nil {
		t.Fatalf("Failed to add validator: %v", err)
	}
	
	// Mark the validator as problematic for a specific request type
	const requestType = "account"
	success := addrDir.MarkValidatorRequestTypeProblematic("peer1", requestType)
	if !success {
		t.Errorf("MarkValidatorRequestTypeProblematic returned false, expected true")
	}
	
	// Verify the validator is marked as problematic for the specific request type
	problematicValidator := addrDir.GetValidator("peer1")
	if problematicValidator == nil {
		t.Fatalf("Failed to retrieve validator")
	}
	
	// Check if the request type is in the problematic types slice
	found := false
	for _, rt := range problematicValidator.ProblematicRequestTypes {
		if rt == requestType {
			found = true
			break
		}
	}
	
	if !found {
		t.Errorf("Expected validator to be marked as problematic for request type '%s', but it wasn't", requestType)
	}
	
	// Check if the validator should be avoided for the specific request type
	should := addrDir.ShouldAvoidValidatorForRequestType("peer1", requestType)
	if !should {
		t.Errorf("Expected ShouldAvoidValidatorForRequestType to return true, got false")
	}
	
	// Check if the validator should be avoided for a different request type
	should = addrDir.ShouldAvoidValidatorForRequestType("peer1", "chain")
	if should {
		t.Errorf("Expected ShouldAvoidValidatorForRequestType to return false for different request type, got true")
	}
	
	// Try to mark a non-existent validator as problematic for a request type
	success = addrDir.MarkValidatorRequestTypeProblematic("non-existent", requestType)
	if success {
		t.Errorf("MarkValidatorRequestTypeProblematic returned true for non-existent validator, expected false")
	}
}

// TestUpdateValidatorHeight tests the UpdateValidatorHeight function
func TestUpdateValidatorHeight(t *testing.T) {
	// Create a new AddressDir instance
	addrDir := NewTestAddressDir()
	
	// Create and add a validator
	validator := TestValidator("peer1", "validator1", "dn", "dn")
	err := addrDir.AddValidator(validator)
	if err != nil {
		t.Fatalf("Failed to add validator: %v", err)
	}
	
	// Update the validator height for a directory network
	const height int64 = 12345
	const isDirectoryNetwork = true
	success := addrDir.UpdateValidatorHeight("peer1", height, isDirectoryNetwork)
	if !success {
		t.Errorf("UpdateValidatorHeight returned false, expected true")
	}
	
	// Verify the height was updated
	updatedValidator := addrDir.GetValidator("peer1")
	if updatedValidator == nil {
		t.Fatalf("Failed to retrieve updated validator")
	}
	
	if updatedValidator.DNHeight != uint64(height) {
		t.Errorf("Expected DNHeight to be %d, got %d", height, updatedValidator.DNHeight)
	}
	
	// Verify the LastUpdated timestamp was updated
	if updatedValidator.LastUpdated.IsZero() {
		t.Errorf("Expected LastUpdated to be set, but it was zero")
	}
	
	// Try to update a non-existent validator's height
	success = addrDir.UpdateValidatorHeight("non-existent", height, isDirectoryNetwork)
	if success {
		t.Errorf("UpdateValidatorHeight returned true for non-existent validator, expected false")
	}
	
	// Test updating the BVN height
	const bvnHeight int64 = 54321
	const isBVN = false
	success = addrDir.UpdateValidatorHeight("peer1", bvnHeight, isBVN)
	if !success {
		t.Errorf("UpdateValidatorHeight for BVN returned false, expected true")
	}
	
	// Verify both heights were updated correctly
	updatedValidator = addrDir.GetValidator("peer1")
	if updatedValidator == nil {
		t.Fatalf("Failed to retrieve updated validator after BVN height update")
	}
	
	if updatedValidator.DNHeight != uint64(height) {
		t.Errorf("Expected DNHeight to be %d, got %d", height, updatedValidator.DNHeight)
	}
	
	if updatedValidator.BVNHeight != uint64(bvnHeight) {
		t.Errorf("Expected BVNHeight to be %d, got %d", bvnHeight, updatedValidator.BVNHeight)
	}
	
	// Verify the BVN height was updated correctly
	if updatedValidator.BVNHeight != uint64(bvnHeight) {
		t.Errorf("Expected BVNHeight to be %d, got %d", bvnHeight, updatedValidator.BVNHeight)
	}
}
