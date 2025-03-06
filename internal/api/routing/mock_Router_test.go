// Code generated by mockery v2.42.3. DO NOT EDIT.

package routing

import (
	mock "github.com/stretchr/testify/mock"
	messaging "gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"

	url "gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// MockRouter is an autogenerated mock type for the Router type
type MockRouter struct {
	mock.Mock
}

type MockRouter_Expecter struct {
	mock *mock.Mock
}

func (_m *MockRouter) EXPECT() *MockRouter_Expecter {
	return &MockRouter_Expecter{mock: &_m.Mock}
}

// Route provides a mock function with given fields: _a0
func (_m *MockRouter) Route(_a0 ...*messaging.Envelope) (string, error) {
	_va := make([]interface{}, len(_a0))
	for _i := range _a0 {
		_va[_i] = _a0[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	if len(ret) == 0 {
		panic("no return value specified for Route")
	}

	var r0 string
	var r1 error
	if rf, ok := ret.Get(0).(func(...*messaging.Envelope) (string, error)); ok {
		return rf(_a0...)
	}
	if rf, ok := ret.Get(0).(func(...*messaging.Envelope) string); ok {
		r0 = rf(_a0...)
	} else {
		r0 = ret.Get(0).(string)
	}

	if rf, ok := ret.Get(1).(func(...*messaging.Envelope) error); ok {
		r1 = rf(_a0...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockRouter_Route_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Route'
type MockRouter_Route_Call struct {
	*mock.Call
}

// Route is a helper method to define mock.On call
//   - _a0 ...*messaging.Envelope
func (_e *MockRouter_Expecter) Route(_a0 ...interface{}) *MockRouter_Route_Call {
	return &MockRouter_Route_Call{Call: _e.mock.On("Route",
		append([]interface{}{}, _a0...)...)}
}

func (_c *MockRouter_Route_Call) Run(run func(_a0 ...*messaging.Envelope)) *MockRouter_Route_Call {
	_c.Call.Run(func(args mock.Arguments) {
		variadicArgs := make([]*messaging.Envelope, len(args)-0)
		for i, a := range args[0:] {
			if a != nil {
				variadicArgs[i] = a.(*messaging.Envelope)
			}
		}
		run(variadicArgs...)
	})
	return _c
}

func (_c *MockRouter_Route_Call) Return(_a0 string, _a1 error) *MockRouter_Route_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockRouter_Route_Call) RunAndReturn(run func(...*messaging.Envelope) (string, error)) *MockRouter_Route_Call {
	_c.Call.Return(run)
	return _c
}

// RouteAccount provides a mock function with given fields: _a0
func (_m *MockRouter) RouteAccount(_a0 *url.URL) (string, error) {
	ret := _m.Called(_a0)

	if len(ret) == 0 {
		panic("no return value specified for RouteAccount")
	}

	var r0 string
	var r1 error
	if rf, ok := ret.Get(0).(func(*url.URL) (string, error)); ok {
		return rf(_a0)
	}
	if rf, ok := ret.Get(0).(func(*url.URL) string); ok {
		r0 = rf(_a0)
	} else {
		r0 = ret.Get(0).(string)
	}

	if rf, ok := ret.Get(1).(func(*url.URL) error); ok {
		r1 = rf(_a0)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockRouter_RouteAccount_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'RouteAccount'
type MockRouter_RouteAccount_Call struct {
	*mock.Call
}

// RouteAccount is a helper method to define mock.On call
//   - _a0 *url.URL
func (_e *MockRouter_Expecter) RouteAccount(_a0 interface{}) *MockRouter_RouteAccount_Call {
	return &MockRouter_RouteAccount_Call{Call: _e.mock.On("RouteAccount", _a0)}
}

func (_c *MockRouter_RouteAccount_Call) Run(run func(_a0 *url.URL)) *MockRouter_RouteAccount_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*url.URL))
	})
	return _c
}

func (_c *MockRouter_RouteAccount_Call) Return(_a0 string, _a1 error) *MockRouter_RouteAccount_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockRouter_RouteAccount_Call) RunAndReturn(run func(*url.URL) (string, error)) *MockRouter_RouteAccount_Call {
	_c.Call.Return(run)
	return _c
}

// NewMockRouter creates a new instance of MockRouter. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockRouter(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockRouter {
	mock := &MockRouter{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
