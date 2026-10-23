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

// Run prepares SandboxArgs using command line parameters and TOML
// config and starts Drop main process.
func Run(flags *RunFlags, homeDir, dropHome string) error {
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

	if err := FlagsToConfig(cfg, flags); err != nil {
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
