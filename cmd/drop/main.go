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

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/wrr/drop/internal/cli"
	"github.com/wrr/drop/internal/jailfs"
	"github.com/wrr/drop/internal/osutil"
	"github.com/wrr/drop/internal/proc"
)

var Version = "" // overridden by release binaries linker

func init() {
	// Keep the main goroutine on the main thread. This is needed for two things:
	//
	// * Subprocesses started by the main goroutine can use the Pdeathsig
	// flag to ensure they are terminated when drop exits.
	//
	// * Goroutines that switch to the sandbox's network namespace
	// never switch the main thread's namespace.
	runtime.LockOSThread()

	if Version == "" {
		// Not a realease binary, but a binary installed from the git repo
		// with `go install` or local development binary.
		if info, ok := debug.ReadBuildInfo(); ok {
			v := strings.TrimPrefix(info.Main.Version, "v")
			if v != "" {
				// Version info present (builder had access to the repo and
				// git tags). This can be a clean version number, or with a
				// '-dirty' suffix.
				Version = v
			} else {
				Version = "dev"
			}
		}
	}
}

func main() {
	var err error
	if len(os.Args) > 1 && os.Args[1] == proc.DropArg(proc.ProcMain) {
		err = proc.Main()
	} else if len(os.Args) > 1 && os.Args[1] == proc.DropArg(proc.ProcInit) {
		err = proc.Init()
	} else {
		err = userCommand()
	}
	exitCode, err := errToExit(err)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
	os.Exit(exitCode)
}

func userCommand() error {
	if os.Geteuid() == 0 {
		return fmt.Errorf("drop should not be run as root")
	}

	homeDir, err := osutil.CurrentUserHomeDir()
	if err != nil {
		return err
	}
	dropHome, err := jailfs.DropHome(homeDir)
	if err != nil {
		return err
	}
	cmd := cli.Command(Version, homeDir, dropHome)
	return cmd.Run(context.Background(), os.Args)
}

func errToExit(err error) (int, error) {
	if err == nil {
		return 0, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		// Propagate child's exit code without printing any error message.
		return exitError.ExitCode(), nil
	}
	return 1, err
}
