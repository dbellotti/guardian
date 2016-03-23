// This file was generated by counterfeiter
package fakes

import (
	"os/exec"
	"sync"

	"github.com/cloudfoundry-incubator/guardian/rundmc/runrunc"
)

type FakeRuncBinary struct {
	StartCommandStub        func(path, id string, detach bool) *exec.Cmd
	startCommandMutex       sync.RWMutex
	startCommandArgsForCall []struct {
		path   string
		id     string
		detach bool
	}
	startCommandReturns struct {
		result1 *exec.Cmd
	}
	ExecCommandStub        func(id, processJSONPath, pidFilePath string) *exec.Cmd
	execCommandMutex       sync.RWMutex
	execCommandArgsForCall []struct {
		id              string
		processJSONPath string
		pidFilePath     string
	}
	execCommandReturns struct {
		result1 *exec.Cmd
	}
	KillCommandStub        func(id, signal string) *exec.Cmd
	killCommandMutex       sync.RWMutex
	killCommandArgsForCall []struct {
		id     string
		signal string
	}
	killCommandReturns struct {
		result1 *exec.Cmd
	}
	StateCommandStub        func(id string) *exec.Cmd
	stateCommandMutex       sync.RWMutex
	stateCommandArgsForCall []struct {
		id string
	}
	stateCommandReturns struct {
		result1 *exec.Cmd
	}
	StatsCommandStub        func(id string) *exec.Cmd
	statsCommandMutex       sync.RWMutex
	statsCommandArgsForCall []struct {
		id string
	}
	statsCommandReturns struct {
		result1 *exec.Cmd
	}
	DeleteCommandStub        func(id string) *exec.Cmd
	deleteCommandMutex       sync.RWMutex
	deleteCommandArgsForCall []struct {
		id string
	}
	deleteCommandReturns struct {
		result1 *exec.Cmd
	}
	EventsCommandStub        func(id string) *exec.Cmd
	eventsCommandMutex       sync.RWMutex
	eventsCommandArgsForCall []struct {
		id string
	}
	eventsCommandReturns struct {
		result1 *exec.Cmd
	}
}

func (fake *FakeRuncBinary) StartCommand(path string, id string, detach bool) *exec.Cmd {
	fake.startCommandMutex.Lock()
	fake.startCommandArgsForCall = append(fake.startCommandArgsForCall, struct {
		path   string
		id     string
		detach bool
	}{path, id, detach})
	fake.startCommandMutex.Unlock()
	if fake.StartCommandStub != nil {
		return fake.StartCommandStub(path, id, detach)
	} else {
		return fake.startCommandReturns.result1
	}
}

func (fake *FakeRuncBinary) StartCommandCallCount() int {
	fake.startCommandMutex.RLock()
	defer fake.startCommandMutex.RUnlock()
	return len(fake.startCommandArgsForCall)
}

func (fake *FakeRuncBinary) StartCommandArgsForCall(i int) (string, string, bool) {
	fake.startCommandMutex.RLock()
	defer fake.startCommandMutex.RUnlock()
	return fake.startCommandArgsForCall[i].path, fake.startCommandArgsForCall[i].id, fake.startCommandArgsForCall[i].detach
}

func (fake *FakeRuncBinary) StartCommandReturns(result1 *exec.Cmd) {
	fake.StartCommandStub = nil
	fake.startCommandReturns = struct {
		result1 *exec.Cmd
	}{result1}
}

func (fake *FakeRuncBinary) ExecCommand(id string, processJSONPath string, pidFilePath string) *exec.Cmd {
	fake.execCommandMutex.Lock()
	fake.execCommandArgsForCall = append(fake.execCommandArgsForCall, struct {
		id              string
		processJSONPath string
		pidFilePath     string
	}{id, processJSONPath, pidFilePath})
	fake.execCommandMutex.Unlock()
	if fake.ExecCommandStub != nil {
		return fake.ExecCommandStub(id, processJSONPath, pidFilePath)
	} else {
		return fake.execCommandReturns.result1
	}
}

func (fake *FakeRuncBinary) ExecCommandCallCount() int {
	fake.execCommandMutex.RLock()
	defer fake.execCommandMutex.RUnlock()
	return len(fake.execCommandArgsForCall)
}

func (fake *FakeRuncBinary) ExecCommandArgsForCall(i int) (string, string, string) {
	fake.execCommandMutex.RLock()
	defer fake.execCommandMutex.RUnlock()
	return fake.execCommandArgsForCall[i].id, fake.execCommandArgsForCall[i].processJSONPath, fake.execCommandArgsForCall[i].pidFilePath
}

func (fake *FakeRuncBinary) ExecCommandReturns(result1 *exec.Cmd) {
	fake.ExecCommandStub = nil
	fake.execCommandReturns = struct {
		result1 *exec.Cmd
	}{result1}
}

func (fake *FakeRuncBinary) KillCommand(id string, signal string) *exec.Cmd {
	fake.killCommandMutex.Lock()
	fake.killCommandArgsForCall = append(fake.killCommandArgsForCall, struct {
		id     string
		signal string
	}{id, signal})
	fake.killCommandMutex.Unlock()
	if fake.KillCommandStub != nil {
		return fake.KillCommandStub(id, signal)
	} else {
		return fake.killCommandReturns.result1
	}
}

func (fake *FakeRuncBinary) KillCommandCallCount() int {
	fake.killCommandMutex.RLock()
	defer fake.killCommandMutex.RUnlock()
	return len(fake.killCommandArgsForCall)
}

func (fake *FakeRuncBinary) KillCommandArgsForCall(i int) (string, string) {
	fake.killCommandMutex.RLock()
	defer fake.killCommandMutex.RUnlock()
	return fake.killCommandArgsForCall[i].id, fake.killCommandArgsForCall[i].signal
}

func (fake *FakeRuncBinary) KillCommandReturns(result1 *exec.Cmd) {
	fake.KillCommandStub = nil
	fake.killCommandReturns = struct {
		result1 *exec.Cmd
	}{result1}
}

func (fake *FakeRuncBinary) StateCommand(id string) *exec.Cmd {
	fake.stateCommandMutex.Lock()
	fake.stateCommandArgsForCall = append(fake.stateCommandArgsForCall, struct {
		id string
	}{id})
	fake.stateCommandMutex.Unlock()
	if fake.StateCommandStub != nil {
		return fake.StateCommandStub(id)
	} else {
		return fake.stateCommandReturns.result1
	}
}

func (fake *FakeRuncBinary) StateCommandCallCount() int {
	fake.stateCommandMutex.RLock()
	defer fake.stateCommandMutex.RUnlock()
	return len(fake.stateCommandArgsForCall)
}

func (fake *FakeRuncBinary) StateCommandArgsForCall(i int) string {
	fake.stateCommandMutex.RLock()
	defer fake.stateCommandMutex.RUnlock()
	return fake.stateCommandArgsForCall[i].id
}

func (fake *FakeRuncBinary) StateCommandReturns(result1 *exec.Cmd) {
	fake.StateCommandStub = nil
	fake.stateCommandReturns = struct {
		result1 *exec.Cmd
	}{result1}
}

func (fake *FakeRuncBinary) StatsCommand(id string) *exec.Cmd {
	fake.statsCommandMutex.Lock()
	fake.statsCommandArgsForCall = append(fake.statsCommandArgsForCall, struct {
		id string
	}{id})
	fake.statsCommandMutex.Unlock()
	if fake.StatsCommandStub != nil {
		return fake.StatsCommandStub(id)
	} else {
		return fake.statsCommandReturns.result1
	}
}

func (fake *FakeRuncBinary) StatsCommandCallCount() int {
	fake.statsCommandMutex.RLock()
	defer fake.statsCommandMutex.RUnlock()
	return len(fake.statsCommandArgsForCall)
}

func (fake *FakeRuncBinary) StatsCommandArgsForCall(i int) string {
	fake.statsCommandMutex.RLock()
	defer fake.statsCommandMutex.RUnlock()
	return fake.statsCommandArgsForCall[i].id
}

func (fake *FakeRuncBinary) StatsCommandReturns(result1 *exec.Cmd) {
	fake.StatsCommandStub = nil
	fake.statsCommandReturns = struct {
		result1 *exec.Cmd
	}{result1}
}

func (fake *FakeRuncBinary) DeleteCommand(id string) *exec.Cmd {
	fake.deleteCommandMutex.Lock()
	fake.deleteCommandArgsForCall = append(fake.deleteCommandArgsForCall, struct {
		id string
	}{id})
	fake.deleteCommandMutex.Unlock()
	if fake.DeleteCommandStub != nil {
		return fake.DeleteCommandStub(id)
	} else {
		return fake.deleteCommandReturns.result1
	}
}

func (fake *FakeRuncBinary) DeleteCommandCallCount() int {
	fake.deleteCommandMutex.RLock()
	defer fake.deleteCommandMutex.RUnlock()
	return len(fake.deleteCommandArgsForCall)
}

func (fake *FakeRuncBinary) DeleteCommandArgsForCall(i int) string {
	fake.deleteCommandMutex.RLock()
	defer fake.deleteCommandMutex.RUnlock()
	return fake.deleteCommandArgsForCall[i].id
}

func (fake *FakeRuncBinary) DeleteCommandReturns(result1 *exec.Cmd) {
	fake.DeleteCommandStub = nil
	fake.deleteCommandReturns = struct {
		result1 *exec.Cmd
	}{result1}
}

func (fake *FakeRuncBinary) EventsCommand(id string) *exec.Cmd {
	fake.eventsCommandMutex.Lock()
	fake.eventsCommandArgsForCall = append(fake.eventsCommandArgsForCall, struct {
		id string
	}{id})
	fake.eventsCommandMutex.Unlock()
	if fake.EventsCommandStub != nil {
		return fake.EventsCommandStub(id)
	} else {
		return fake.eventsCommandReturns.result1
	}
}

func (fake *FakeRuncBinary) EventsCommandCallCount() int {
	fake.eventsCommandMutex.RLock()
	defer fake.eventsCommandMutex.RUnlock()
	return len(fake.eventsCommandArgsForCall)
}

func (fake *FakeRuncBinary) EventsCommandArgsForCall(i int) string {
	fake.eventsCommandMutex.RLock()
	defer fake.eventsCommandMutex.RUnlock()
	return fake.eventsCommandArgsForCall[i].id
}

func (fake *FakeRuncBinary) EventsCommandReturns(result1 *exec.Cmd) {
	fake.EventsCommandStub = nil
	fake.eventsCommandReturns = struct {
		result1 *exec.Cmd
	}{result1}
}

var _ runrunc.RuncBinary = new(FakeRuncBinary)
