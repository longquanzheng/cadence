// The MIT License (MIT)

// Copyright (c) 2017-2020 Uber Technologies Inc.

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

// Code generated by MockGen. DO NOT EDIT.
// Source: controller.go

// Package shard is a generated GoMock package.
package shard

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"

	engine "github.com/uber/cadence/service/history/engine"
)

// MockEngineFactory is a mock of EngineFactory interface
type MockEngineFactory struct {
	ctrl     *gomock.Controller
	recorder *MockEngineFactoryMockRecorder
}

// MockEngineFactoryMockRecorder is the mock recorder for MockEngineFactory
type MockEngineFactoryMockRecorder struct {
	mock *MockEngineFactory
}

// NewMockEngineFactory creates a new mock instance
func NewMockEngineFactory(ctrl *gomock.Controller) *MockEngineFactory {
	mock := &MockEngineFactory{ctrl: ctrl}
	mock.recorder = &MockEngineFactoryMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockEngineFactory) EXPECT() *MockEngineFactoryMockRecorder {
	return m.recorder
}

// CreateEngine mocks base method
func (m *MockEngineFactory) CreateEngine(arg0 Context) engine.Engine {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateEngine", arg0)
	ret0, _ := ret[0].(engine.Engine)
	return ret0
}

// CreateEngine indicates an expected call of CreateEngine
func (mr *MockEngineFactoryMockRecorder) CreateEngine(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateEngine", reflect.TypeOf((*MockEngineFactory)(nil).CreateEngine), arg0)
}

// MockController is a mock of Controller interface
type MockController struct {
	ctrl     *gomock.Controller
	recorder *MockControllerMockRecorder
}

// MockControllerMockRecorder is the mock recorder for MockController
type MockControllerMockRecorder struct {
	mock *MockController
}

// NewMockController creates a new mock instance
func NewMockController(ctrl *gomock.Controller) *MockController {
	mock := &MockController{ctrl: ctrl}
	mock.recorder = &MockControllerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockController) EXPECT() *MockControllerMockRecorder {
	return m.recorder
}

// Start mocks base method
func (m *MockController) Start() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Start")
}

// Start indicates an expected call of Start
func (mr *MockControllerMockRecorder) Start() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Start", reflect.TypeOf((*MockController)(nil).Start))
}

// Stop mocks base method
func (m *MockController) Stop() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Stop")
}

// Stop indicates an expected call of Stop
func (mr *MockControllerMockRecorder) Stop() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Stop", reflect.TypeOf((*MockController)(nil).Stop))
}

// PrepareToStop mocks base method
func (m *MockController) PrepareToStop() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "PrepareToStop")
}

// PrepareToStop indicates an expected call of PrepareToStop
func (mr *MockControllerMockRecorder) PrepareToStop() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "PrepareToStop", reflect.TypeOf((*MockController)(nil).PrepareToStop))
}

// GetEngine mocks base method
func (m *MockController) GetEngine(workflowID string) (engine.Engine, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetEngine", workflowID)
	ret0, _ := ret[0].(engine.Engine)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetEngine indicates an expected call of GetEngine
func (mr *MockControllerMockRecorder) GetEngine(workflowID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetEngine", reflect.TypeOf((*MockController)(nil).GetEngine), workflowID)
}

// GetEngineForShard mocks base method
func (m *MockController) GetEngineForShard(shardID int) (engine.Engine, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetEngineForShard", shardID)
	ret0, _ := ret[0].(engine.Engine)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetEngineForShard indicates an expected call of GetEngineForShard
func (mr *MockControllerMockRecorder) GetEngineForShard(shardID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetEngineForShard", reflect.TypeOf((*MockController)(nil).GetEngineForShard), shardID)
}

// RemoveEngineForShard mocks base method
func (m *MockController) RemoveEngineForShard(shardID int) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "RemoveEngineForShard", shardID)
}

// RemoveEngineForShard indicates an expected call of RemoveEngineForShard
func (mr *MockControllerMockRecorder) RemoveEngineForShard(shardID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RemoveEngineForShard", reflect.TypeOf((*MockController)(nil).RemoveEngineForShard), shardID)
}

// Status mocks base method
func (m *MockController) Status() int32 {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Status")
	ret0, _ := ret[0].(int32)
	return ret0
}

// Status indicates an expected call of Status
func (mr *MockControllerMockRecorder) Status() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Status", reflect.TypeOf((*MockController)(nil).Status))
}

// NumShards mocks base method
func (m *MockController) NumShards() int {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NumShards")
	ret0, _ := ret[0].(int)
	return ret0
}

// NumShards indicates an expected call of NumShards
func (mr *MockControllerMockRecorder) NumShards() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NumShards", reflect.TypeOf((*MockController)(nil).NumShards))
}

// ShardIDs mocks base method
func (m *MockController) ShardIDs() []int32 {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ShardIDs")
	ret0, _ := ret[0].([]int32)
	return ret0
}

// ShardIDs indicates an expected call of ShardIDs
func (mr *MockControllerMockRecorder) ShardIDs() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ShardIDs", reflect.TypeOf((*MockController)(nil).ShardIDs))
}
