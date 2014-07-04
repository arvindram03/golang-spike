// Copyright 2013, Google Inc. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sync2

import (
	"sync"
)

// These are the three predefined states of a service.
const (
	SERVICE_STOPPED = iota
	SERVICE_RUNNING
	SERVICE_SHUTTING_DOWN
)

var stateNames = []string{
	"Stopped",
	"Running",
	"ShuttingDown",
}

// ServiceManager manages the state of a service
// through its lifecycle.
type ServiceManager struct {
	mu    sync.Mutex
	wg    sync.WaitGroup
	state AtomicInt64
}

// Go tries to change the state from SERVICE_STOPPED to SERVICE_RUNNING.
// If the current state is not SERVICE_STOPPED (already running),
// it returns false immediately.
// On successful transition, it launches the service as a goroutine and returns true.
// The service func is required to regularly check the state of the service manager.
// If the state is not SERVICE_RUNNING, it must treat it as end of service and return.
// When the service func returns, the state is reverted to SERVICE_STOPPED.
func (svm *ServiceManager) Go(service func(svm *ServiceManager)) bool {
	svm.mu.Lock()
	defer svm.mu.Unlock()
	if !svm.state.CompareAndSwap(SERVICE_STOPPED, SERVICE_RUNNING) {
		return false
	}
	svm.wg.Add(1)
	go func() {
		service(svm)
		svm.state.Set(SERVICE_STOPPED)
		svm.wg.Done()
	}()
	return true
}

// Stop tries to change the state from SERVICE_RUNNING to SERVICE_SHUTTING_DOWN.
// If the current state is not SERVICE_RUNNING, it returns false immediately.
// On successul transition, it waits for the service to finish, and returns true.
// You are allowed to 'Go' again after a Stop.
func (svm *ServiceManager) Stop() bool {
	svm.mu.Lock()
	defer svm.mu.Unlock()
	if !svm.state.CompareAndSwap(SERVICE_RUNNING, SERVICE_SHUTTING_DOWN) {
		return false
	}
	svm.wg.Wait()
	return true
}

// Wait waits for the service to terminate if it's currently running.
func (svm *ServiceManager) Wait() {
	svm.wg.Wait()
}

// State returns the current state of the service.
// This should only be used to report the current state.
func (svm *ServiceManager) State() int64 {
	return svm.state.Get()
}

// StateName returns the name of the current state.
func (svm *ServiceManager) StateName() string {
	return stateNames[svm.State()]
}
