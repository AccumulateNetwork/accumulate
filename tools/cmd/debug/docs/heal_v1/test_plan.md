# Test Plan: URL Normalization and Routing Conflict Resolution

This test plan outlines the approach for verifying that our URL normalization solution resolves the "cannot route message" errors in the healing process.

## Test Cases

### 1. URL Normalization Function Tests

**Test Case 1.1: Basic URL Normalization**
- **Input**: Anchor pool URL (`acc://dn.acme/anchors/Apollo`)
- **Expected Output**: Partition URL (`acc://bvn-Apollo.acme`)
- **Verification**: The function correctly converts between URL formats

**Test Case 1.2: Edge Cases**
- **Input**: Various malformed URLs
- **Expected Output**: Graceful handling without panics
- **Verification**: The function handles edge cases properly

### 2. Routing Conflict Resolution Tests

**Test Case 2.1: Submission with Conflicting Routes**
- **Setup**: Create a message with URLs that would cause routing conflicts
- **Action**: Submit through the modified submitLoop function
- **Expected Result**: Message is normalized and submitted successfully
- **Verification**: No "conflicting routes" error is returned

**Test Case 2.2: Caching Mechanism**
- **Setup**: Submit the same message multiple times
- **Expected Result**: First submission is processed, subsequent ones are skipped
- **Verification**: Log shows "Skipping cached submission" for duplicates

### 3. Integration Tests

**Test Case 3.1: End-to-End Healing Process**
- **Setup**: Run the healing process on a test network with known gaps
- **Action**: Observe the healing process with the modified code
- **Expected Result**: All gaps are healed without routing errors
- **Verification**: Logs show successful submissions and no routing conflicts

**Test Case 3.2: Performance Comparison**
- **Setup**: Run healing with and without the modifications
- **Action**: Measure completion time and error rates
- **Expected Result**: Modified version has fewer errors and similar or better performance
- **Verification**: Metrics show improvement in success rate

## Test Implementation

### Unit Tests

```go
func TestNormalizeUrl(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
    }{
        {
            name:     "Anchor Pool URL",
            input:    "acc://dn.acme/anchors/Apollo",
            expected: "acc://bvn-Apollo.acme",
        },
        {
            name:     "Already Normalized URL",
            input:    "acc://bvn-Apollo.acme",
            expected: "acc://bvn-Apollo.acme",
        },
        {
            name:     "Non-Anchor URL",
            input:    "acc://user.acme/tokens",
            expected: "acc://user.acme/tokens",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            inputUrl, err := url.Parse(tt.input)
            require.NoError(t, err)
            
            normalized := normalizeUrl(inputUrl)
            assert.Equal(t, tt.expected, normalized.String())
        })
    }
}
```

### Integration Test

```go
func TestHealingWithUrlNormalization(t *testing.T) {
    // Setup test network with known gaps
    network := setupTestNetwork(t)
    
    // Run healing process
    h := new(healer)
    h.setup(context.Background(), network.Name)
    
    // Track routing errors
    var routingErrors int
    h.errorHandler = func(err error) {
        if strings.Contains(err.Error(), "conflicting routes") {
            routingErrors++
        }
    }
    
    // Run healing
    h.healAll([]string{network.Name})
    
    // Verify no routing errors occurred
    assert.Equal(t, 0, routingErrors, "Expected no routing errors")
    
    // Verify all gaps were healed
    gaps := countRemainingGaps(t, network)
    assert.Equal(t, 0, gaps, "Expected all gaps to be healed")
}
```

## Manual Testing

1. Deploy the modified code to a test environment
2. Run the healing process with verbose logging enabled
3. Monitor logs for any "conflicting routes" errors
4. Verify that messages are successfully submitted
5. Check that the healing process completes without errors

## Success Criteria

The implementation will be considered successful if:

1. No "cannot route message" errors occur during the healing process
2. All anchor gaps are successfully healed
3. The solution works consistently across all partition types
4. Performance is maintained or improved compared to the original implementation

## Rollback Plan

If issues are encountered:

1. Revert to the original submitLoop implementation
2. Document the specific failures for further analysis
3. Consider alternative approaches to URL normalization
