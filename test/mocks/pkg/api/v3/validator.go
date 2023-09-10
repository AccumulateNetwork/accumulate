// Code generated by mockery v2.23.1. DO NOT EDIT.

package mocks

import (
	context "context"

	mock "github.com/stretchr/testify/mock"
	api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	messaging "gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

// Validator is an autogenerated mock type for the Validator type
type Validator struct {
	mock.Mock
}

type Validator_Expecter struct {
	mock *mock.Mock
}

func (_m *Validator) EXPECT() *Validator_Expecter {
	return &Validator_Expecter{mock: &_m.Mock}
}

// Validate provides a mock function with given fields: ctx, envelope, opts
func (_m *Validator) Validate(ctx context.Context, envelope *messaging.Envelope, opts api.ValidateOptions) ([]*api.Submission, error) {
	ret := _m.Called(ctx, envelope, opts)

	var r0 []*api.Submission
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *messaging.Envelope, api.ValidateOptions) ([]*api.Submission, error)); ok {
		return rf(ctx, envelope, opts)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *messaging.Envelope, api.ValidateOptions) []*api.Submission); ok {
		r0 = rf(ctx, envelope, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*api.Submission)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *messaging.Envelope, api.ValidateOptions) error); ok {
		r1 = rf(ctx, envelope, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Validator_Validate_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Validate'
type Validator_Validate_Call struct {
	*mock.Call
}

// Validate is a helper method to define mock.On call
//   - ctx context.Context
//   - envelope *messaging.Envelope
//   - opts api.ValidateOptions
func (_e *Validator_Expecter) Validate(ctx interface{}, envelope interface{}, opts interface{}) *Validator_Validate_Call {
	return &Validator_Validate_Call{Call: _e.mock.On("Validate", ctx, envelope, opts)}
}

func (_c *Validator_Validate_Call) Run(run func(ctx context.Context, envelope *messaging.Envelope, opts api.ValidateOptions)) *Validator_Validate_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(*messaging.Envelope), args[2].(api.ValidateOptions))
	})
	return _c
}

func (_c *Validator_Validate_Call) Return(_a0 []*api.Submission, _a1 error) *Validator_Validate_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *Validator_Validate_Call) RunAndReturn(run func(context.Context, *messaging.Envelope, api.ValidateOptions) ([]*api.Submission, error)) *Validator_Validate_Call {
	_c.Call.Return(run)
	return _c
}

type mockConstructorTestingTNewValidator interface {
	mock.TestingT
	Cleanup(func())
}

// NewValidator creates a new instance of Validator. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewValidator(t mockConstructorTestingTNewValidator) *Validator {
	mock := &Validator{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
