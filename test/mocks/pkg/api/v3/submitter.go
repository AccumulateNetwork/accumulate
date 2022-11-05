// Code generated by mockery v2.14.0. DO NOT EDIT.

package mocks

import (
	context "context"

	mock "github.com/stretchr/testify/mock"
	api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	protocol "gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Submitter is an autogenerated mock type for the Submitter type
type Submitter struct {
	mock.Mock
}

type Submitter_Expecter struct {
	mock *mock.Mock
}

func (_m *Submitter) EXPECT() *Submitter_Expecter {
	return &Submitter_Expecter{mock: &_m.Mock}
}

// Submit provides a mock function with given fields: ctx, envelope, opts
func (_m *Submitter) Submit(ctx context.Context, envelope *protocol.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
	ret := _m.Called(ctx, envelope, opts)

	var r0 []*api.Submission
	if rf, ok := ret.Get(0).(func(context.Context, *protocol.Envelope, api.SubmitOptions) []*api.Submission); ok {
		r0 = rf(ctx, envelope, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*api.Submission)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, *protocol.Envelope, api.SubmitOptions) error); ok {
		r1 = rf(ctx, envelope, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Submitter_Submit_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Submit'
type Submitter_Submit_Call struct {
	*mock.Call
}

// Submit is a helper method to define mock.On call
//  - ctx context.Context
//  - envelope *protocol.Envelope
//  - opts api.SubmitOptions
func (_e *Submitter_Expecter) Submit(ctx interface{}, envelope interface{}, opts interface{}) *Submitter_Submit_Call {
	return &Submitter_Submit_Call{Call: _e.mock.On("Submit", ctx, envelope, opts)}
}

func (_c *Submitter_Submit_Call) Run(run func(ctx context.Context, envelope *protocol.Envelope, opts api.SubmitOptions)) *Submitter_Submit_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(*protocol.Envelope), args[2].(api.SubmitOptions))
	})
	return _c
}

func (_c *Submitter_Submit_Call) Return(_a0 []*api.Submission, _a1 error) *Submitter_Submit_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

type mockConstructorTestingTNewSubmitter interface {
	mock.TestingT
	Cleanup(func())
}

// NewSubmitter creates a new instance of Submitter. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewSubmitter(t mockConstructorTestingTNewSubmitter) *Submitter {
	mock := &Submitter{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
