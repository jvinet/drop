// Copyright 2026 Jan Wrobel <jan@mixedbit.org>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package proc is responsible for starting Drop sub processes.
// Process hierarchy of 'drop run' is:
//
//	drop run                          started by the user
//	 |                                waits for `drop -main`
//	 |
//	 +- drop -main                    new user namespace
//	     |                            sets up the network, forwards
//	     |                            the PTY, supervises init
//	     |
//	     +- drop -init                new user, mount, PID, net, IPC
//	     |                            and cgroup namespaces, PID 1
//	     |                            exec sandboxed program
//	     |
//	     +- pasta (daemonized)        joins the sandbox net namespace
//
// `drop -main` process is needed for the firewall and DNS logic to
// be able to join the network namespace of the sandbox. Joining a
// network namespace requires the process to have CAP_SYS_ADMIN in its
// own namespace, so the process needs to run in a new user namespace.
//
// Technically, when firewall is not used, 'drop -main' logic could
// run directly in the 'drop run' process, without spawning 'drop
// -main'. But we keep using the same hierarchy in all cases to limit
// the complexity by making different Drop configurations use as
// similar execution paths as possible.
package proc

import (
	"fmt"

	"golang.org/x/sys/unix"
	"kernel.org/pub/linux/libs/security/libcap/cap"

	"github.com/wrr/drop/internal/ipc"
)

type ProcType string

const (
	ProcMain ProcType = "main"
	ProcInit ProcType = "init"
)

// DropArg returns a command line argument to be passed to `drop` to
// start a process of a given ProcType
func DropArg(procType ProcType) string {
	return "-" + string(procType)
}

// StartMain starts a `drop -main` sub process as a child of the
// current process, sends args to it via a Unix socket and waits for
// the sub process to exit.
//
// It also ensures the child dies if the parent process dies and
// forwards terminating signals to the child.
func StartMain(args ipc.SandboxArgs) error {
	cmd := nsCommand(ProcMain)
	defer cmd.cleanup()

	cmd.ambientCaps = []uintptr{unix.CAP_SYS_ADMIN}
	// The main process killAfter time needs to be longer than the one
	// used for the init process. If sandboxed process eats terminating
	// signals, we want sandbox (init) to be killed first, before the
	// main process is killed. This way main can properly restore the
	// terminal.
	cmd.killAfter = 2 * initKillAfter

	if err := cmd.start(); err != nil {
		return err
	}
	if err := cmd.sendArgs(args); err != nil {
		return err
	}
	return cmd.wait()
}

// ensureCapSysAdmin returns an error if process doesn't have
// CAP_SYS_ADMIN capability. Ubuntu restricts user namespaces creation
// via apparmor profiles, but programs that are not allowed to create
// user namespaces, can in fact create them but without capabilities
// that make the namespace usable (clone system call succeeds, child
// process runs and has different user namespace from its parent as
// indicated by a different id in /proc/self/ns/user). To detect
// situation when creating a namespace is blocked by app armor
// profile, we test for a presence of CAP_SYS_ADMIN.
func ensureCapSysAdmin() error {
	caps := cap.GetProc()
	hasCap, err := caps.GetFlag(cap.Effective, unix.CAP_SYS_ADMIN)
	if err != nil {
		return fmt.Errorf("check CAP_SYS_ADMIN capability: %v", err)
	}
	if !hasCap {
		return fmt.Errorf("not enough capabilities. Are Linux user namespaces enabled? " +
			"Is Drop allowed to use user namespaces via AppArmor profile?")
	}
	return nil
}

func dropAllCaps() error {
	old := cap.GetProc()
	empty := cap.NewSet()
	if err := empty.SetProc(); err != nil {
		return fmt.Errorf("drop privilege: %q -> %q: %v", old, empty, err)
	}
	now := cap.GetProc()
	if cf, _ := now.Cf(empty); cf != 0 {
		return fmt.Errorf("privileges not fully dropped: have=%q, wanted=%q", now, empty)
	}
	return nil
}
