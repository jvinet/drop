// Copyright 2025 Jan Wrobel <jan@mixedbit.org>
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

// Package ipc provides communication between processes that setup
// Drop sandbox. The communication is via a Unix domain socket.
package ipc

import (
	"encoding/gob"
	"fmt"
	"os"

	"golang.org/x/sys/unix"

	"github.com/wrr/drop/internal/config"
	"github.com/wrr/drop/internal/jailfs"
)

// SandboxArgs contains arguments needed by the sandbox that the Drop
// main process constructs and sends. This allows command line
// argument parsing and TOML configs reading and parsing to be done
// once by a main process, and ensures processes executed by the main
// process see the same args.
//
// Note for future extensions: unexported fields or interface types if
// included within SandboxArgs hierarchy will not be encoded and sent
// (encoding/gob limitation).
type SandboxArgs struct {
	EnvId    string
	Paths    *jailfs.Paths
	Config   *config.Config
	ExecArgs []string
}

type ArgsSender struct {
	socket *os.File
}

type ArgsReceiver struct {
	// Public, so it can be passed to child processes as ExtraFiles
	Socket *os.File
}

// NewArgsSocket creates an anonymous socket for sending sandbox
// arguments.
func NewArgsSocket() (*ArgsSender, *ArgsReceiver, error) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("create socket pair: %v", err)
	}
	return NewArgsSender(uintptr(fds[0])), NewArgsReceiver(uintptr(fds[1])), nil
}

func NewArgsSender(fd uintptr) *ArgsSender {
	return &ArgsSender{
		socket: os.NewFile(fd, "sandbox-args-sender-socket"),
	}
}

func NewArgsReceiver(fd uintptr) *ArgsReceiver {
	return &ArgsReceiver{
		Socket: os.NewFile(fd, "sandbox-args-receiver-socket"),
	}
}

// Send serializes and sends all the necessary arguments and
// configuration options obtained by the main process from command
// line and from config files.
func (p *ArgsSender) Send(args SandboxArgs) error {
	if err := gob.NewEncoder(p.socket).Encode(args); err != nil {
		return fmt.Errorf("send arguments: %v", err)
	}
	return nil
}

func (p *ArgsSender) Close() error {
	if p.socket != nil {
		err := p.socket.Close()
		p.socket = nil
		return err
	}
	return nil
}

// Recv receives arguments. The function blocks until the arguments
// are available.
func (c *ArgsReceiver) Recv() (*SandboxArgs, error) {
	args := SandboxArgs{}
	if err := gob.NewDecoder(c.Socket).Decode(&args); err != nil {
		return nil, fmt.Errorf("receive arguments: %v", err)
	}
	return &args, nil
}

func (c *ArgsReceiver) Close() error {
	if c.Socket != nil {
		err := c.Socket.Close()
		c.Socket = nil
		return err
	}
	return nil
}
