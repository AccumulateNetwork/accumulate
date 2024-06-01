// Code generated by mockery v2.42.3. DO NOT EDIT.

package dial

import (
	context "context"

	mock "github.com/stretchr/testify/mock"
	message "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
)

// MockConnector is an autogenerated mock type for the Connector type
type MockConnector struct {
	mock.Mock
}

type MockConnector_Expecter struct {
	mock *mock.Mock
}

func (_m *MockConnector) EXPECT() *MockConnector_Expecter {
	return &MockConnector_Expecter{mock: &_m.Mock}
}

// Connect provides a mock function with given fields: _a0, _a1
func (_m *MockConnector) Connect(_a0 context.Context, _a1 *ConnectionRequest) (message.StreamOf[message.Message], error) {
	ret := _m.Called(_a0, _a1)

	if len(ret) == 0 {
		panic("no return value specified for Connect")
	}

	var r0 message.StreamOf[message.Message]
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *ConnectionRequest) (message.StreamOf[message.Message], error)); ok {
		return rf(_a0, _a1)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *ConnectionRequest) message.StreamOf[message.Message]); ok {
		r0 = rf(_a0, _a1)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(message.StreamOf[message.Message])
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *ConnectionRequest) error); ok {
		r1 = rf(_a0, _a1)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockConnector_Connect_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Connect'
type MockConnector_Connect_Call struct {
	*mock.Call
}

// Connect is a helper method to define mock.On call
//   - _a0 context.Context
//   - _a1 *ConnectionRequest
func (_e *MockConnector_Expecter) Connect(_a0 interface{}, _a1 interface{}) *MockConnector_Connect_Call {
	return &MockConnector_Connect_Call{Call: _e.mock.On("Connect", _a0, _a1)}
}

func (_c *MockConnector_Connect_Call) Run(run func(_a0 context.Context, _a1 *ConnectionRequest)) *MockConnector_Connect_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(*ConnectionRequest))
	})
	return _c
}

func (_c *MockConnector_Connect_Call) Return(_a0 message.StreamOf[message.Message], _a1 error) *MockConnector_Connect_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockConnector_Connect_Call) RunAndReturn(run func(context.Context, *ConnectionRequest) (message.StreamOf[message.Message], error)) *MockConnector_Connect_Call {
	_c.Call.Return(run)
	return _c
}

// NewMockConnector creates a new instance of MockConnector. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockConnector(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockConnector {
	mock := &MockConnector{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
