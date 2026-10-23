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

// 'drop run ...' command handling

package cli

import (
	"fmt"

	"github.com/wrr/drop/internal/config"
	"github.com/wrr/drop/internal/ipc"
	"github.com/wrr/drop/internal/jailfs"
	"github.com/wrr/drop/internal/osutil"
	"github.com/wrr/drop/internal/proc"
)

// runFlags contains parsed command line flags for the 'drop run'
// command.
type runFlags struct {
	EnvId       string
	Runtime     string
	ConfigPath  string
	NetworkMode string

	Mounts []string

	BeRoot            bool
	TcpPublishedPorts []string
	TcpHostPorts      []string
	UdpPublishedPorts []string
	UdpHostPorts      []string

	// Remaining command line arguments (the command to execute)
	Args []string
}

// run prepares SandboxArgs using command line parameters and TOML
// config and starts Drop main process.
func run(flags *runFlags, homeDir, dropHome string) error {
	var configPath string
	if !osutil.CanStat(jailfs.EnvPath(dropHome, flags.EnvId)) {
		return fmt.Errorf("environment %q doesn't exist, run 'drop init %v' to create it", flags.EnvId, flags.EnvId)
	}
	if flags.ConfigPath == "" {
		configPath = jailfs.EnvConfigPath(homeDir, flags.EnvId)
	} else {
		configPath = flags.ConfigPath
	}

	cfg, err := config.Read(configPath, homeDir)
	if err != nil {
		return err
	}

	if err := flagsToConfig(cfg, flags); err != nil {
		return err
	}

	if (len(flags.TcpPublishedPorts) > 0 ||
		len(flags.TcpHostPorts) > 0 ||
		len(flags.UdpPublishedPorts) > 0 ||
		len(flags.UdpHostPorts) > 0) &&
		cfg.Net.Mode != config.NetModeIsolated {
		return fmt.Errorf("port forwarding is only supported with isolated network mode (--net isolated)")
	}

	paths, cleanup, err := jailfs.NewPaths(homeDir, flags.EnvId)
	if err != nil {
		return err
	}
	defer cleanup()

	sandboxArgs := ipc.SandboxArgs{
		EnvId:    flags.EnvId,
		Paths:    paths,
		Config:   cfg,
		BeRoot:   flags.BeRoot,
		ExecArgs: flags.Args,
	}
	return proc.StartMain(sandboxArgs)
}

// flagsToConfig modifies cfg from a TOML file with values passed via
// command line flags. The function validates config after the
// modification.
func flagsToConfig(cfg *config.Config, flags *runFlags) error {
	for _, m := range flags.Mounts {
		mount, err := config.ParseMountCompact(m)
		if err != nil {
			return fmt.Errorf("command line --mount flag: %v", err)
		}
		cfg.Mounts = append(cfg.Mounts, *mount)
	}

	if flags.NetworkMode != "" {
		cfg.Net.Mode = flags.NetworkMode
	}
	if flags.Runtime != "" {
		cfg.Runtime = flags.Runtime
	}
	if len(flags.TcpPublishedPorts) > 0 {
		p, err := parsePublishPortFlags(flags.TcpPublishedPorts)
		if err != nil {
			return fmt.Errorf("command line --tcp-publish flag: %v", err)
		}
		cfg.Net.TCPPublishedPorts = append(cfg.Net.TCPPublishedPorts, p...)
	}
	if len(flags.TcpHostPorts) > 0 {
		p, err := parseHostPortFlags(flags.TcpHostPorts)
		if err != nil {
			return fmt.Errorf("command line --tcp-host flag: %v", err)
		}
		cfg.Net.TCPHostPorts = append(cfg.Net.TCPHostPorts, p...)
	}
	if len(flags.UdpPublishedPorts) > 0 {
		p, err := parsePublishPortFlags(flags.UdpPublishedPorts)
		if err != nil {
			return fmt.Errorf("command line --udp-publish flag: %v", err)
		}
		cfg.Net.UDPPublishedPorts = append(cfg.Net.UDPPublishedPorts, p...)
	}
	if len(flags.UdpHostPorts) > 0 {
		p, err := parseHostPortFlags(flags.UdpHostPorts)
		if err != nil {
			return fmt.Errorf("command line --udp-host flag: %v", err)
		}
		cfg.Net.UDPHostPorts = append(cfg.Net.UDPHostPorts, p...)
	}
	// Validate config again, all errors detected should be related to
	// entries modified by this function, because cfg read from a file
	// and passed to this function was already validated during reading.
	if err := config.Validate(cfg); err != nil {
		return fmt.Errorf("command line flags: %v", err)
	}
	return nil
}

func parsePublishPortFlags(flags []string) ([]config.PublishedPort, error) {
	var result []config.PublishedPort
	for _, spec := range flags {
		p, err := config.ParsePublishedPort(spec)
		if err != nil {
			return nil, err
		}
		result = append(result, *p)
	}
	return result, nil
}

func parseHostPortFlags(flags []string) ([]config.HostPort, error) {
	var result []config.HostPort
	for _, spec := range flags {
		p, err := config.ParseHostPort(spec)
		if err != nil {
			return nil, err
		}
		result = append(result, *p)
	}
	return result, nil
}
