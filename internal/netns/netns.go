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

// Package netns performs operations in the network namespace of
// another process (the sandbox).
//
// The functions require CAP_SYS_ADMIN in the caller's user namespace
// and in the user namespace that owns the target network namespace
// (setns(2)).
//
// Each operation runs in a new goroutine that locks its OS thread and
// switches the thread to the target network namespace. The thread is
// never switched back. Switching would require CAP_SYS_ADMIN in the
// user namespace owning the original network namespace, which for
// Drop is the initial user namespace (host). Instead, the goroutine
// exits with the thread still locked, and the runtime terminates the
// thread.
//
// The main thread can't be terminated, so it must not run these
// goroutines: the main goroutine must be locked to the main thread
// with runtime.LockOSThread() in an init function.
package netns

import (
	"fmt"
	"net"
	"net/netip"
	"os"
	"runtime"

	"github.com/wrr/drop/internal/osutil"
	"golang.org/x/sys/unix"
)

// ListenLoopbackUDP returns a UDP socket bound to 127.0.0.1:port in
// the network namespace of the process pid. The socket stays in that
// namespace, while the caller and all other threads keep using the
// current one.
//
// The main thread of the process must be locked to the main goroutine.
func ListenLoopbackUDP(pid int, port uint16) (net.PacketConn, error) {
	nsPath := fmt.Sprintf("/proc/%d/ns/net", pid)
	ns, err := os.Open(nsPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %v", nsPath, err)
	}
	defer ns.Close()

	type result struct {
		conn net.PacketConn
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		// Drop can't setns back to the original network namespace, so the
		// thread stays locked after setns. When a goroutine exits locked,
		// the runtime terminates its thread instead of reusing it. The
		// main thread can't be terminated and would stay in the target
		// namespace.
		runtime.LockOSThread()
		if osutil.IsMainThread() {
			runtime.UnlockOSThread()
			ch <- result{nil, fmt.Errorf("enter network namespace: main thread not locked to main goroutine")}
			return
		}
		if err := unix.Setns(int(ns.Fd()), unix.CLONE_NEWNET); err != nil {
			runtime.UnlockOSThread()
			ch <- result{nil, fmt.Errorf("enter network namespace: %v", err)}
			return
		}
		addr := netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), port)
		conn, err := net.ListenUDP("udp4", net.UDPAddrFromAddrPort(addr))
		if err != nil {
			ch <- result{nil, fmt.Errorf("listen on %v in network namespace: %v", addr, err)}
			return
		}
		ch <- result{conn, nil}
	}()
	r := <-ch
	return r.conn, r.err
}
