// This file was generated by counterfeiter
package fakes

import (
	"sync"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/guardian/gardener"
)

type FakeContainerizer struct {
	CreateStub        func(spec gardener.DesiredContainerSpec) error
	createMutex       sync.RWMutex
	createArgsForCall []struct {
		spec gardener.DesiredContainerSpec
	}
	createReturns struct {
		result1 error
	}
	RunStub        func(handle string, spec garden.ProcessSpec, io garden.ProcessIO) (garden.Process, error)
	runMutex       sync.RWMutex
	runArgsForCall []struct {
		handle string
		spec   garden.ProcessSpec
		io     garden.ProcessIO
	}
	runReturns struct {
		result1 garden.Process
		result2 error
	}
}

func (fake *FakeContainerizer) Create(spec gardener.DesiredContainerSpec) error {
	fake.createMutex.Lock()
	fake.createArgsForCall = append(fake.createArgsForCall, struct {
		spec gardener.DesiredContainerSpec
	}{spec})
	fake.createMutex.Unlock()
	if fake.CreateStub != nil {
		return fake.CreateStub(spec)
	} else {
		return fake.createReturns.result1
	}
}

func (fake *FakeContainerizer) CreateCallCount() int {
	fake.createMutex.RLock()
	defer fake.createMutex.RUnlock()
	return len(fake.createArgsForCall)
}

func (fake *FakeContainerizer) CreateArgsForCall(i int) gardener.DesiredContainerSpec {
	fake.createMutex.RLock()
	defer fake.createMutex.RUnlock()
	return fake.createArgsForCall[i].spec
}

func (fake *FakeContainerizer) CreateReturns(result1 error) {
	fake.CreateStub = nil
	fake.createReturns = struct {
		result1 error
	}{result1}
}

func (fake *FakeContainerizer) Run(handle string, spec garden.ProcessSpec, io garden.ProcessIO) (garden.Process, error) {
	fake.runMutex.Lock()
	fake.runArgsForCall = append(fake.runArgsForCall, struct {
		handle string
		spec   garden.ProcessSpec
		io     garden.ProcessIO
	}{handle, spec, io})
	fake.runMutex.Unlock()
	if fake.RunStub != nil {
		return fake.RunStub(handle, spec, io)
	} else {
		return fake.runReturns.result1, fake.runReturns.result2
	}
}

func (fake *FakeContainerizer) RunCallCount() int {
	fake.runMutex.RLock()
	defer fake.runMutex.RUnlock()
	return len(fake.runArgsForCall)
}

func (fake *FakeContainerizer) RunArgsForCall(i int) (string, garden.ProcessSpec, garden.ProcessIO) {
	fake.runMutex.RLock()
	defer fake.runMutex.RUnlock()
	return fake.runArgsForCall[i].handle, fake.runArgsForCall[i].spec, fake.runArgsForCall[i].io
}

func (fake *FakeContainerizer) RunReturns(result1 garden.Process, result2 error) {
	fake.RunStub = nil
	fake.runReturns = struct {
		result1 garden.Process
		result2 error
	}{result1, result2}
}

var _ gardener.Containerizer = new(FakeContainerizer)
