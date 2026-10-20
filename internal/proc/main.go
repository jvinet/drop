// Copyright 2025-2026 Jan Wrobel <jan@mixedbit.org>
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
	"syscall"

	"golang.org/x/sys/unix"
	"golang.org/x/term"

	"github.com/wrr/drop/internal/config"
	"github.com/wrr/drop/internal/ipc"
	"github.com/wrr/drop/internal/netns"
	"github.com/wrr/drop/internal/pty"
)

// Main handles main process logic for the 'drop run' command.
func Main() error {
	argsReceiver := ipc.NewArgsReceiver(3)

	args, err := argsReceiver.Recv()
	argsReceiver.Close()
	if err != nil {
		return err
	}
	paths := args.Paths
	cfg := args.Config

	if err := ensureCapSysAdmin(); err != nil {
		return err
	}

	cmd := nsCommand(ProcInit)
	defer cmd.cleanup()

	// 1) For each stdin|out|err which is a terminal, we pass it as-is
	// to the init process, so it is also able to detect that fd is a
	// terminal. The terminal is then replaced with a new PTY created in
	// the sandbox.
	//
	// 2) For each stdin|out|err which is not a terminal, a pipe is
	// created and passed to the sandboxed process and a goroutine is
	// started to pass data between the original descriptor and this
	// pipe. This way, the original file is not passed directly to the
	// sandboxed process, and the sandboxed process cannot access and
	// modify it via /proc/self/fd/0|1|2.
	//
	// For stdout and stderr this is done by wrapping the original file
	// with the io.Writer interface, which makes cmd replace it with a
	// pipe and copy from it in a goroutine.
	//
	// Stdin is not wrapped this way, instead the copying is done by a
	// goroutine started by nsCmd. The reason is that cmd.Wait waits for
	// each goroutine it created before returning. If sandboxed process
	// terminates, we want drop to also terminate, not to wait for its
	// stdin to be closed first (for stdout|err we actually need to
	// wait, these descriptors are for sure closed when program
	// terminates and we want Drop to show any pending output).
	if !term.IsTerminal(0) {
		cmd.pipeStdin = true
	}
	if !term.IsTerminal(1) {
		cmd.stdout = struct{ io.Writer }{os.Stdout}
	}
	if !term.IsTerminal(2) {
		cmd.stderr = struct{ io.Writer }{os.Stderr}
	}
	cmd.beRoot = args.BeRoot
	cmd.killAfter = initKillAfter
	cmd.namespaces = syscall.CLONE_NEWNS |
		syscall.CLONE_NEWIPC |
		syscall.CLONE_NEWPID |
		syscall.CLONE_NEWCGROUP |
		syscall.CLONE_NEWNET

	// CAP_DAC_OVERRIDE and CAP_FOWNER are needed to mount overlayfs
	cmd.ambientCaps = []uintptr{
		unix.CAP_SYS_ADMIN,
		unix.CAP_DAC_OVERRIDE,
		unix.CAP_FOWNER,
	}
	if err := cmd.start(); err != nil {
		return err
	}

	var ptyReceiver *ipc.PtyReceiver
	if pty.PtyNeeded() {
		ptyReceiver, err = ipc.NewPtyReceiver(paths.PtySocket)
		if err != nil {
			return err
		}
		defer ptyReceiver.Close()
	}
	// Don't pass extra capabilities to pasta
	// Firewall and DNS setup will be done before caps are dropped
	if err := dropAllCaps(); err != nil {
		return err
	}

	// Start pasta to provide network connectivity to the sandboxed process
	// in isolated network mode
	if cfg.Net.Mode == config.NetModeIsolated {
		// Store Pasta pid and log files in tmp run dir, not in the main
		// paths.Run, because the default AppArmor policy shipped with pasta
		// and used on AppArmor-enabled Debian and openSUSE, as well as
		// SELinux policy on Fedora, allow write access to /tmp but do not
		// allow access to the user home.
		cleanPasta, err := netns.StartPasta(cmd.pid(), cfg.Net, paths.RunInTmp)
		if err != nil {
			return err
		}
		defer cleanPasta()
	}
	// This must be done after network setup is finished. Args sending
	// is used inform the init process that the network setup is done
	// and a sandboxed program can be executed.
	if err := cmd.sendArgs(*args); err != nil {
		return err
	}

	type ptyResult struct {
		parentPty *os.File
		err       error
	}

	ptyCh := make(chan ptyResult, 1)
	if ptyReceiver != nil {
		// Run concurrently because it can block in case the child dies
		// before sending the pty.
		go func() {
			parentPty, err := ptyReceiver.Recv()
			ptyCh <- ptyResult{parentPty, err}
		}()
	}

	var ptyError error
	childCh := cmd.waitCh()

	for {
		select {
		case err := <-childCh:
			if ptyError != nil && !errors.Is(ptyError, io.EOF) {
				// EOF means child terminated, most likely due to some non
				// pty related reason, in all other cases ptyError takes
				// priority as it likely was the cause of the child failure.
				return fmt.Errorf("pty setup: %v", ptyError)
			}
			return err
		case result := <-ptyCh:
			ptyError = result.err
			if ptyError == nil {
				var cleanForwardPty func()
				cleanForwardPty, ptyError = pty.ForwardPty(result.parentPty)
				if cleanForwardPty != nil {
					defer cleanForwardPty()
				}
			}
			if ptyError != nil {
				// We continue to wait for the child to exit in this select
				// loop.
				cmd.kill()
			}
		}
	}
}
