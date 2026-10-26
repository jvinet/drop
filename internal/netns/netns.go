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

// Package netns runs code in the network namespace of another process
// (the sandbox).
package netns

import (
	"fmt"
	"os"
	"runtime"

	"github.com/wrr/drop/internal/osutil"
	"golang.org/x/sys/unix"
)

// Do executes fn in the network namespace of the process pid
// without switching the calling goroutine's network namespace.
//
// Sockets that fn creates belong to the target network namespace for
// their whole lifetime, even when they are later used from threads in
// the original namespace.
//
// Do requires CAP_SYS_ADMIN in the caller's user namespace
// and in the user namespace that owns the target network namespace
// (setns(2)).
//
// To use this function the main thread of the process must be locked
// to the main goroutine (LockOSThread called from init()).
func Do(pid int, fn func() error) error {
	nsPath := fmt.Sprintf("/proc/%d/ns/net", pid)
	ns, err := os.Open(nsPath)
	if err != nil {
		return fmt.Errorf("open %s: %v", nsPath, err)
	}
	defer ns.Close()

	ch := make(chan error, 1)
	// fn is executed in a new goroutine that locks its OS thread and
	// switches the thread to the target network namespace. The thread is
	// never switched back. Switching would require CAP_SYS_ADMIN in the
	// user namespace owning the original network namespace, which for
	// Drop is the initial user namespace (host). Instead, the goroutine
	// exits with the thread still locked, and the runtime terminates the
	// thread.
	//
	// To ensure this goroutine is not scheduled on the main thread,
	// (main thread can't be terminated and would linger unused until
	// program termination), the main goroutine should be locked to the
	// main thread.
	go func() {
		runtime.LockOSThread()
		if osutil.IsMainThread() {
			runtime.UnlockOSThread()
			ch <- fmt.Errorf("enter network namespace: main thread not locked to main goroutine")
			return
		}
		if err := unix.Setns(int(ns.Fd()), unix.CLONE_NEWNET); err != nil {
			runtime.UnlockOSThread()
			ch <- fmt.Errorf("enter network namespace: %v", err)
			return
		}
		ch <- fn()
	}()
	return <-ch
}
