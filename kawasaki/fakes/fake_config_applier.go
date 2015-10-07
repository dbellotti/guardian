// This file was generated by counterfeiter
package fakes

import (
	"sync"

	"github.com/cloudfoundry-incubator/guardian/kawasaki"
)

type FakeConfigApplier struct {
	ApplyStub        func(cfg kawasaki.NetworkConfig, nsPath string) error
	applyMutex       sync.RWMutex
	applyArgsForCall []struct {
		cfg    kawasaki.NetworkConfig
		nsPath string
	}
	applyReturns struct {
		result1 error
	}
}

func (fake *FakeConfigApplier) Apply(cfg kawasaki.NetworkConfig, nsPath string) error {
	fake.applyMutex.Lock()
	fake.applyArgsForCall = append(fake.applyArgsForCall, struct {
		cfg    kawasaki.NetworkConfig
		nsPath string
	}{cfg, nsPath})
	fake.applyMutex.Unlock()
	if fake.ApplyStub != nil {
		return fake.ApplyStub(cfg, nsPath)
	} else {
		return fake.applyReturns.result1
	}
}

func (fake *FakeConfigApplier) ApplyCallCount() int {
	fake.applyMutex.RLock()
	defer fake.applyMutex.RUnlock()
	return len(fake.applyArgsForCall)
}

func (fake *FakeConfigApplier) ApplyArgsForCall(i int) (kawasaki.NetworkConfig, string) {
	fake.applyMutex.RLock()
	defer fake.applyMutex.RUnlock()
	return fake.applyArgsForCall[i].cfg, fake.applyArgsForCall[i].nsPath
}

func (fake *FakeConfigApplier) ApplyReturns(result1 error) {
	fake.ApplyStub = nil
	fake.applyReturns = struct {
		result1 error
	}{result1}
}

var _ kawasaki.ConfigApplier = new(FakeConfigApplier)