// Code generated by MockGen. DO NOT EDIT.
// Source: gopkg.in/juju/charmrepo.v4 (interfaces: Interface)

// Package mocks is a generated GoMock package.
package mocks

import (
	gomock "github.com/golang/mock/gomock"
	charm_v6 "gopkg.in/juju/charm.v6"
	reflect "reflect"
)

// MockInterface is a mock of Interface interface
type MockInterface struct {
	ctrl     *gomock.Controller
	recorder *MockInterfaceMockRecorder
}

// MockInterfaceMockRecorder is the mock recorder for MockInterface
type MockInterfaceMockRecorder struct {
	mock *MockInterface
}

// NewMockInterface creates a new mock instance
func NewMockInterface(ctrl *gomock.Controller) *MockInterface {
	mock := &MockInterface{ctrl: ctrl}
	mock.recorder = &MockInterfaceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockInterface) EXPECT() *MockInterfaceMockRecorder {
	return m.recorder
}

// Get mocks base method
func (m *MockInterface) Get(arg0 *charm_v6.URL, arg1 string) (*charm_v6.CharmArchive, error) {
	ret := m.ctrl.Call(m, "Get", arg0, arg1)
	ret0, _ := ret[0].(*charm_v6.CharmArchive)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Get indicates an expected call of Get
func (mr *MockInterfaceMockRecorder) Get(arg0, arg1 interface{}) *gomock.Call {
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Get", reflect.TypeOf((*MockInterface)(nil).Get), arg0, arg1)
}

// GetBundle mocks base method
func (m *MockInterface) GetBundle(arg0 *charm_v6.URL, arg1 string) (charm_v6.Bundle, error) {
	ret := m.ctrl.Call(m, "GetBundle", arg0, arg1)
	ret0, _ := ret[0].(charm_v6.Bundle)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetBundle indicates an expected call of GetBundle
func (mr *MockInterfaceMockRecorder) GetBundle(arg0, arg1 interface{}) *gomock.Call {
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetBundle", reflect.TypeOf((*MockInterface)(nil).GetBundle), arg0, arg1)
}

// Resolve mocks base method
func (m *MockInterface) Resolve(arg0 *charm_v6.URL) (*charm_v6.URL, []string, error) {
	ret := m.ctrl.Call(m, "Resolve", arg0)
	ret0, _ := ret[0].(*charm_v6.URL)
	ret1, _ := ret[1].([]string)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// Resolve indicates an expected call of Resolve
func (mr *MockInterfaceMockRecorder) Resolve(arg0 interface{}) *gomock.Call {
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Resolve", reflect.TypeOf((*MockInterface)(nil).Resolve), arg0)
}
