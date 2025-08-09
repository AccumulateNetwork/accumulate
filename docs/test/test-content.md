# Accumulate Network Test Suite Documentation

## Overview

The Accumulate Network test suite is a comprehensive testing framework that covers all aspects of the blockchain network, from basic transaction processing to complex network operations. The tests are organized into several categories, each focusing on specific functionality areas.

## Test Directory Structure

```
test/
├── cmd/                    # Command-line test utilities (13 items)
├── data/                   # Test data and benchmarks (10 items)
├── docs/                   # Test documentation (12 files)
├── e2e/                    # End-to-end integration tests (53 files)
├── e2e_v2/                   # Additional E2E tests (5 files)
├── encoding/               # Encoding/decoding tests (5 files)
├── harness/                # Test harness and utilities (5 files)
├── helpers/                # Test helper functions (1 file)
├── mocks/                  # Mock implementations (10 files)
├── sdk/                    # SDK tests (2 files)
├── simulator/              # Network simulator (43 files)
├── testdata/               # Static test data (11 files)
├── testing/                # Testing utilities and frameworks (14 files)
├── util/                   # Test utilities (6 files)
└── validate/               # Validation tests (3 files)
```

## Test Categories

### 1. End-to-End (E2E) Tests (`test/e2e/`)

The E2E tests are the most comprehensive, testing complete workflows and interactions:

#### API Tests
- **api_test.go**: Tests API functionality including minor block expansion and query operations

#### Transaction Tests
- **txn_send_tokens_test.go**: Token transfer operations, duplicate recipients
- **txn_add_credits_test.go**: Credit addition, ACME burning and refunding
- **txn_burn_credits_test.go**: Credit burning operations
- **txn_create_identity_test.go**: Identity creation workflows
- **txn_create_lite_data_account_test.go**: Lite data account creation
- **txn_create_lite_token_account_test.go**: Lite token account creation
- **txn_create_token_account_test.go**: Standard token account creation
- **txn_expire_test.go**: Transaction expiration handling
- **txn_issue_tokens_test.go**: Token issuance operations
- **txn_lock_account_test.go**: Account locking mechanisms
- **txn_refund_test.go**: Transaction refund processes, failed transaction handling
- **txn_transfer_credits_test.go**: Credit transfer operations
- **txn_update_account_auth_test.go**: Account authority updates, signature requests
- **txn_update_key_test.go**: Key update operations
- **txn_write_data_test.go**: Data writing operations
- **txn_write_data2_test.go**: Advanced data writing scenarios

#### Signature Tests
- **sig_additional_test.go**: Additional authority signatures, missing authority handling
- **sig_advanced_test.go**: Advanced signing scenarios, block holds, priority handling, expiration
- **sig_delegated_test.go**: Delegated signature operations
- **sig_eip712_test.go**: EIP-712 signature standard compliance
- **sig_general_test.go**: General signature validation
- **sig_inherit_test.go**: Signature inheritance mechanisms
- **sig_multi_test.go**: Multi-signature operations

#### Network Tests
- **net_anchors_test.go**: Anchor dropping, directory anchor signature reuse
- **net_globals_test.go**: Global network parameter management
- **net_globals2_test.go**: Advanced global parameter scenarios
- **net_maintenance_test.go**: Network maintenance operations
- **net_major_block_test.go**: Major block processing
- **net_stall_test.go**: Network stall detection and recovery
- **net_validators_test.go**: Validator management and updates

#### Block and Execution Tests
- **block_test.go**: Block processing and validation
- **exec_process_test.go**: Transaction execution processes
- **exec_synthetic_test.go**: Synthetic transaction handling
- **major_block_test.go**: Major block operations
- **msg_block_anchor_test.go**: Block anchor message processing

#### System Tests
- **genesis_test.go**: Genesis block creation, activation routes, routing
- **limits_test.go**: System limits and constraints
- **credits_test.go**: Credit accounting and management
- **fuzz_test.go**: Fuzz testing for robustness
- **replay_test.go**: Transaction replay scenarios
- **state_consistency_test.go**: State consistency validation
- **sys_block_test.go**: System block operations

#### Regression Tests
- **regression_test.go**: Critical bug regression tests including:
  - Credit balance overwrites
  - Remote authority key queries
  - Cross-BVN operations
  - Synthetic transaction handling
  - Multi-network faucet operations
  - Directory transaction routing
  - Cross-partition delegation and authority
  - Missing account handling
  - DN anchor acknowledgment

- **regression2_test.go**: Additional regression test scenarios

#### Sequence Tests
- **sequence_test.go**: Transaction sequencing, out-of-order handling
- **sequence2_test.go**: Advanced sequencing scenarios

#### Validator Tests
- **validators_test.go**: Validator operations and management

#### Special Tests
- **_factom_addresses_test.go**: Factom address migration testing (prefixed with underscore)
- **_relaunch_test.go**: Network relaunch scenarios (prefixed with underscore)
- **util_test.go**: E2E test utilities and helper functions

### 2. Command-Line Test Utilities (`test/cmd/`)

#### Load Testing
- **load/main.go**: Load testing utility for performance evaluation

#### Development Tools
- **devnet/main.go**: Development network setup and testing
- **gen-testdata/**: Test data generation utilities
- **_manual/**: Manual testing scripts and utilities

#### Ethereum Integration
- **eth_signTypedData/**: Ethereum EIP-712 signature testing
  - JavaScript-based testing framework
  - Execute scripts for signature validation
  - Package management for Node.js dependencies

### 3. Test Simulator (`test/simulator/`)

The simulator provides a complete network simulation environment:

#### Core Components
- **simulator.go**: Main simulator implementation
- **factory.go**: Network factory for creating test networks
- **partition.go**: Partition simulation
- **node.go**: Node simulation
- **consensus.go**: Consensus mechanism simulation
- **api.go**: API simulation layer

#### Supporting Infrastructure
- **dispatcher.go**: Message dispatching
- **router.go**: Message routing simulation
- **faucet.go**: Test token faucet
- **signing.go**: Signature simulation
- **recorder.go**: Event recording and playback
- **task_queue.go**: Asynchronous task management

#### Configuration
- **options.go**: Simulator configuration options
- **order_test.go**: Transaction ordering tests (317KB - extensive)

### 4. Test Harness (`test/harness/`)

Provides testing infrastructure and utilities:

- **harness.go**: Main test harness framework
- **conditions.go**: Test condition definitions and validation
- **query.go**: Query testing utilities
- **simulator.go**: Simulator integration
- **failure.go**: Failure handling and reporting

### 5. Testing Utilities (`test/testing/`)

Core testing infrastructure:

- **node.go**: Test node implementations
- **state.go**: State management for tests
- **batch.go**: Batch operation testing
- **router.go**: Routing test utilities
- **observer.go**: Test observers
- **fake_types.go**: Mock type implementations
- **httpshim.go**: HTTP testing shim
- **ip.go**: IP address utilities
- **debug.go**: Debug utilities
- **skip.go**: Test skipping logic

### 6. Mock Implementations (`test/mocks/`)

Mock objects for isolated testing:
- Database mocks
- Network mocks
- API mocks
- Service mocks

### 7. Validation Tests (`test/validate/`)

Input validation and constraint testing:
- Parameter validation
- Data format validation
- Business rule validation

### 8. Encoding Tests (`test/encoding/`)

Data encoding/decoding validation:
- Protocol buffer encoding
- JSON encoding
- Binary encoding
- Hash function testing

### 9. SDK Tests (`test/sdk/`)

Software Development Kit testing:
- Client library tests
- API wrapper tests
- Integration examples

## Test Data and Benchmarks (`test/data/`)

### Performance Benchmarks
- **pkg_database_values_BenchmarkNotFound/**: Database performance benchmarks
  - CPU profiling data
  - Memory profiling data
  - Benchmark results
  - Performance regression tracking

### Debug Data
- **debugtx.txt**: Debug transaction data for troubleshooting

## Key Testing Features

### 1. **Comprehensive Coverage**
- Unit tests for individual components
- Integration tests for component interactions
- End-to-end tests for complete workflows
- Performance and load testing
- Regression testing for critical bugs

### 2. **Network Simulation**
- Complete blockchain network simulation
- Multi-partition testing
- Consensus mechanism testing
- Cross-partition transaction testing
- Network failure simulation

### 3. **Transaction Testing**
- All transaction types covered
- Success and failure scenarios
- Edge cases and error conditions
- Performance under load
- Concurrent transaction handling

### 4. **Signature Testing**
- Multiple signature algorithms
- Multi-signature scenarios
- Delegated signatures
- Authority management
- EIP-712 compliance

### 5. **API Testing**
- REST API endpoints
- JSON-RPC functionality
- WebSocket connections
- Query operations
- Error handling

### 6. **Performance Testing**
- Load testing utilities
- Benchmark suites
- Memory usage profiling
- CPU performance analysis
- Network throughput testing

## Test Execution Patterns

### 1. **Simulator-Based Tests**
Most E2E tests use the simulator framework:
```go
sim := simulator.New(t, 3)  // 3-partition network
sim.InitFromGenesis()
// Test operations...
```

### 2. **Harness-Based Tests**
Complex scenarios use the test harness:
```go
// Setup test environment
// Execute operations
// Validate results
```

### 3. **Mock-Based Tests**
Unit tests use mocks for isolation:
```go
// Create mocks
// Test specific functionality
// Verify interactions
```

## Critical Test Scenarios

### 1. **Cross-Partition Operations**
- Token transfers between partitions
- Authority delegation across partitions
- Anchor message processing
- Synthetic transaction routing

### 2. **Network Resilience**
- Node failures and recovery
- Network partitions
- Consensus failures
- Message delivery guarantees

### 3. **Security Testing**
- Signature validation
- Authority verification
- Access control
- Replay attack prevention

### 4. **Performance Validation**
- Transaction throughput
- Query response times
- Memory usage patterns
- Network bandwidth utilization

## Test Maintenance

### 1. **Regression Prevention**
- Comprehensive regression test suite
- Automated test execution
- Performance regression detection
- Bug reproduction scenarios

### 2. **Test Data Management**
- Generated test data
- Static test fixtures
- Benchmark data preservation
- Debug information collection

### 3. **Continuous Integration**
- Automated test execution
- Performance monitoring
- Test result reporting
- Failure analysis

## Conclusion

The Accumulate Network test suite represents a comprehensive testing framework that ensures the reliability, performance, and security of the blockchain network. With over 200 test functions covering every aspect of the system, from basic operations to complex network scenarios, the test suite provides confidence in the system's robustness and correctness.

The combination of unit tests, integration tests, end-to-end tests, performance tests, and regression tests creates a multi-layered validation approach that catches issues at every level of the system. The sophisticated simulation framework allows for testing complex network scenarios that would be difficult or impossible to reproduce in a live environment.

This testing infrastructure is essential for maintaining the quality and reliability of the Accumulate Network as it continues to evolve and scale.
