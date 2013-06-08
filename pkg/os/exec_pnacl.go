// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build pnacl

package os

import (
	"syscall"
	"time"
)

// The only signal values guaranteed to be present on all systems
// are Interrupt (send the process an interrupt) and Kill (force
// the process to exit).
var (
	Interrupt Signal = syscall.SIGINT
	Kill      Signal = syscall.SIGKILL
)

func startProcess(name string, argv []string, attr *ProcAttr) (p *Process, err error) {
	return nil, syscall.EPERM
}

func (p *Process) kill() error {
	return syscall.EPERM
}

// ProcessState stores information about a process, as reported by Wait.
type ProcessState struct{}

// Pid returns the process id of the exited process.
func (p *ProcessState) Pid() int {
	panic("unimplemented")
}

func (p *ProcessState) exited() bool {
	panic("unimplemented")
}

func (p *ProcessState) success() bool {
	panic("unimplemented")
}

func (p *ProcessState) sys() interface{} {
	panic("unimplemented")
}

func (p *ProcessState) sysUsage() interface{} {
	panic("unimplemented")
}

func (p *Process) wait() (ps *ProcessState, err error) {
	panic("unimplemented")
}

func (p *Process) signal(sig Signal) error {
	panic("unimplemented")
}

func (p *Process) release() error {
	panic("unimplemented")
}

func findProcess(pid int) (p *Process, err error) {
	panic("unimplemented")
}

func (p *ProcessState) userTime() time.Duration {
	panic("unimplemented")
}

func (p *ProcessState) systemTime() time.Duration {
	panic("unimplemented")
}
