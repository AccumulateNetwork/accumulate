// Code generated by mockery v2.14.0. DO NOT EDIT.

package mocks

import (
	context "context"

	mock "github.com/stretchr/testify/mock"
	api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
)

// EventService is an autogenerated mock type for the EventService type
type EventService struct {
	mock.Mock
}

type EventService_Expecter struct {
	mock *mock.Mock
}

func (_m *EventService) EXPECT() *EventService_Expecter {
	return &EventService_Expecter{mock: &_m.Mock}
}

// Subscribe provides a mock function with given fields: ctx, opts
func (_m *EventService) Subscribe(ctx context.Context, opts api.SubscribeOptions) (<-chan api.Event, error) {
	ret := _m.Called(ctx, opts)

	var r0 <-chan api.Event
	if rf, ok := ret.Get(0).(func(context.Context, api.SubscribeOptions) <-chan api.Event); ok {
		r0 = rf(ctx, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(<-chan api.Event)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, api.SubscribeOptions) error); ok {
		r1 = rf(ctx, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// EventService_Subscribe_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Subscribe'
type EventService_Subscribe_Call struct {
	*mock.Call
}

// Subscribe is a helper method to define mock.On call
//   - ctx context.Context
//   - opts api.SubscribeOptions
func (_e *EventService_Expecter) Subscribe(ctx interface{}, opts interface{}) *EventService_Subscribe_Call {
	return &EventService_Subscribe_Call{Call: _e.mock.On("Subscribe", ctx, opts)}
}

func (_c *EventService_Subscribe_Call) Run(run func(ctx context.Context, opts api.SubscribeOptions)) *EventService_Subscribe_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(api.SubscribeOptions))
	})
	return _c
}

func (_c *EventService_Subscribe_Call) Return(_a0 <-chan api.Event, _a1 error) *EventService_Subscribe_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

type mockConstructorTestingTNewEventService interface {
	mock.TestingT
	Cleanup(func())
}

// NewEventService creates a new instance of EventService. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewEventService(t mockConstructorTestingTNewEventService) *EventService {
	mock := &EventService{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}