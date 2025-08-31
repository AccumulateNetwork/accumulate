# Interfaces Directory

This directory contains all interface definitions for TDD development in the Accumulate project.

## Purpose

Following TDD principles, interfaces are defined first before implementation. This directory serves as the contract specification for all features and components.

## Structure

```
internal/interfaces/
├── README.md              # This file
├── core/                  # Core business logic interfaces
├── storage/               # Data storage interfaces  
├── network/               # Network communication interfaces
├── api/                   # API service interfaces
└── common/                # Shared/common interfaces
```

## Guidelines

### Interface Design
- Define clear, focused interfaces
- Use meaningful names that describe behavior
- Keep interfaces small and cohesive (Interface Segregation Principle)
- Document all methods with clear docstrings

### TDD Process
1. **Design Phase**: Create interface in this directory
2. **Test Phase**: Write tests against the interface in `*_test.go` files
3. **Implementation Phase**: Implement the interface in `internal/impl/`

### Naming Conventions
- Use descriptive names ending with interface purpose:
  - `Service` for business logic: `UserService`
  - `Repository` for data access: `UserRepository` 
  - `Client` for external communication: `PaymentClient`
  - `Handler` for request processing: `WebhookHandler`

### File Organization
- One interface per file when complex
- Group related simple interfaces in themed files
- Use package-level documentation for context

## Example Interface

```go
// UserService defines operations for user management
type UserService interface {
    // CreateUser creates a new user account
    CreateUser(ctx context.Context, user *User) (*User, error)
    
    // GetUser retrieves a user by ID
    GetUser(ctx context.Context, id string) (*User, error)
    
    // UpdateUser updates an existing user
    UpdateUser(ctx context.Context, id string, user *User) error
    
    // DeleteUser removes a user account
    DeleteUser(ctx context.Context, id string) error
}
```

## Mock Usage

**IMPORTANT**: Mocks should NEVER be placed in this directory or any production code directory.

- ✅ Create mocks in `*_test.go` files only
- ✅ Use interfaces from this directory for dependency injection
- ❌ Never put mocks in `internal/interfaces/`, `internal/impl/`, `cmd/`, or `pkg/`

## Testing

All interfaces should be thoroughly tested:

```go
// In feature_test.go
type MockUserService struct {
    mock.Mock
}

func (m *MockUserService) CreateUser(ctx context.Context, user *User) (*User, error) {
    args := m.Called(ctx, user)
    return args.Get(0).(*User), args.Error(1)
}
```

## Integration

Implementations should be placed in:
- `internal/impl/[feature]/` - Core implementations
- `cmd/[service]/` - Service entrypoints (no mocks allowed)
- `pkg/[library]/` - Reusable packages (no mocks allowed)

## Validation

Use the TDD validation tools to ensure compliance:

```bash
# Check for mock usage violations
./scripts/tdd/detect_mocks.sh

# Verify test coverage
./scripts/tdd/verify_coverage.sh

# Run complete TDD validation
./scripts/tdd/tdd_validate.sh
```