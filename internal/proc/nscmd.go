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

package proc

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/wrr/drop/internal/ipc"
	"github.com/wrr/drop/internal/osutil"
)

// nsCmd configures and starts drop subprocesses in new namespaces.
//
// All methods need to be called from the main goroutine.
type nsCmd struct {
	stdin     io.Reader
	stdout    io.Writer
	stderr    io.Writer
	beRoot    bool
	pipeStdin bool
	// How much time the child has after the parent forwards a
	// terminating signal to it. After this time elapses the child is
	// killed with SIGKILL.
	killAfter time.Duration
	// In addition to these namespaces, cmd is always executed in a new
	// user namespace.
	namespaces  int
	ambientCaps []uintptr

	// Callers shouldn't access these
	procType          ProcType
	cmd               *exec.Cmd
	argsSender        *ipc.ArgsSender
	processTerminated func()
	waiting           bool
}

// nsCommand returns nsCmd to configure and start a process of type
// procType.
func nsCommand(procType ProcType) *nsCmd {
	return &nsCmd{
		stdin:  os.Stdin,
		stdout: os.Stdout,
		stderr: os.Stderr,

		procType: procType,
	}
}

// Starts a sub process. Registers signal handlers for catchable,
// terminating signal and forwards these signals to the child. Kills
// the child in case the forwarded terminating signals do not
// terminate it within nsCmd.killAfter. Ensures the child is killed
// when the parent dies.
//
// Must be called from the main goroutine locked to the main thread.
func (c *nsCmd) start() error {
	// Ensure Pdeathsig fires when the whole process exits
	// (https://github.com/golang/go/issues/27505).
	if !osutil.IsMainThread() {
		return fmt.Errorf("%s process start: not called from the main goroutine", c.procType)
	}
	exePath, err := dropExePath()
	if err != nil {
		return err
	}
	if c.cmd != nil {
		panic("cmd should not be set")
	}
	cmd := exec.Command(exePath, DropArg(c.procType))
	if c.pipeStdin {
		if err := pipeStdin(cmd, c.stdin); err != nil {
			return err
		}
	} else {
		cmd.Stdin = c.stdin
	}
	cmd.Stdout = c.stdout
	cmd.Stderr = c.stderr
	// Socket pair for sending arguments and synchronizing setup with
	// the child.
	argsSender, argsReceiver, err := ipc.NewArgsSocket()
	if err != nil {
		return err
	}
	defer func() {
		argsReceiver.Close()
		if argsSender != nil {
			argsSender.Close()
		}
	}()
	cmd.ExtraFiles = []*os.File{argsReceiver.Socket}

	cloneFlags := uintptr(syscall.CLONE_NEWUSER | c.namespaces)

	var containerUID, containerGID int
	if c.beRoot {
		containerUID = 0
		containerGID = 0
	} else {
		// Keep using current user uid and gid in the namespace (so the
		// user is recognized as the same user)
		containerUID = os.Getuid()
		containerGID = os.Getgid()
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: cloneFlags,
		// Code running in a user namespace just after clone() and before
		// execve() system calls has all capabilities in this namespace.
		// This allows such code to setup the namespace with mount()
		// and pivot_root() calls.
		// Unfortunately, GO doesn't allow to execute any user provided
		// code between clone() and execve()
		// (https://github.com/golang/go/issues/12125)
		//
		// The first process executed within the namespace with execve no
		// longer has any capabilities in the namespace (unless it has
		// root uid). We need to pass required capabilities to this
		// process. For the detailed description of how capabilities are
		// propagated in the user namespace see: man 7 user_namespaces
		AmbientCaps: c.ambientCaps,
		UidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: containerUID,
				HostID:      os.Getuid(),
				Size:        1,
			},
		},
		GidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: containerGID,
				HostID:      os.Getgid(),
				Size:        1,
			},
		},
		// Disallow a process in the user namespace from dropping/changing
		// group membership: https://lwn.net/Articles/626665/
		GidMappingsEnableSetgroups: false,
		// Kill child process, when this process is killed.
		Pdeathsig: syscall.SIGKILL,
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s process start: %v", c.procType, err)
	}
	c.cmd = cmd
	// Catch catchable shutdown signals, so deferred cleanups run before
	// the process exits. Forward the signals to the child to make sure
	// it exits before the parent in majority of cases. Pdeathsig above
	// is the safety net to kill the child in case parent dies without
	// terminating the child (for example SIGKILL).
	c.processTerminated = osutil.CatchAndForwardSignals(c.cmd.Process, c.killAfter)
	c.argsSender = argsSender
	argsSender = nil
	return nil
}

func (c *nsCmd) sendArgs(args ipc.SandboxArgs) error {
	if c.argsSender == nil {
		panic("send args called twice")
	}
	sender := c.argsSender
	defer sender.Close()
	c.argsSender = nil
	return sender.Send(args)
}

func (c *nsCmd) pid() int {
	return c.cmd.Process.Pid
}

func (c *nsCmd) kill() error {
	return c.cmd.Process.Kill()
}

// wait waits for the child to exit. Can be called only once and
// cannot be called if waitCh() was already called.
func (c *nsCmd) wait() error {
	return <-c.waitCh()
}

// waitCh starts waiting for the child and returns a channel that
// receives its exit error or nil when the child terminates.
func (c *nsCmd) waitCh() <-chan error {
	if c.waiting {
		panic("wait called twice")
	}
	c.waiting = true
	ch := make(chan error, 1)
	go func() {
		err := c.cmd.Wait()
		if err != nil {
			// ExitError causes parent to exit with the child's exit code
			// without printing any error message
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				err = fmt.Errorf("%s process run: %v", c.procType, err)
			}
		}
		ch <- err
	}()
	return ch
}

func (c *nsCmd) cleanup() {
	if c.argsSender != nil {
		c.argsSender.Close()
	}
	if !c.waiting && c.cmd != nil {
		c.cmd.Process.Kill()
		c.wait()
	}
	if c.processTerminated != nil {
		c.processTerminated()
		c.processTerminated = nil
	}
}

// dropExePath returns a path to the drop binary.
// /proc/self/exe is not used because its execution can be denied by
// an AppArmor profile (Ubuntu) and then fails with an error:
// fork/exec /proc/self/exe: permission denied
func dropExePath() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("obtain drop executable path: %v", err)
	}
	return exePath, nil
}

// pipeStdin starts a goroutine that copies src to the started process input.
func pipeStdin(cmd *exec.Cmd, src io.Reader) error {
	pipe, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("create stdin pipe: %v", err)
	}
	go func() {
		io.Copy(pipe, src)
		pipe.Close()
	}()
	return nil
}
