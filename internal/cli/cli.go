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

// Package cli is responsible for Drop command line arguments parsing
// and processing.
package cli

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/wrr/drop/internal/jailfs"
)

// Command creates the urfave/cli command with all commands and flags configured.
func Command(version, homeDir, dropHome string) *cli.Command {
	defaultEnvId, _ := jailfs.CwdToEnvId()
	var flags runFlags
	return &cli.Command{
		Name:      "drop",
		Usage:     "Run programs in sandboxed environments",
		UsageText: "drop <command> [options]",
		Version:   version,
		ExitErrHandler: func(_ context.Context, _ *cli.Command, err error) {
			// blank to avoid the call to os.Exit which drop makes explicitly in main
		},
		Commands: []*cli.Command{
			{
				Name:      "init",
				Usage:     "Create a new Drop environment",
				ArgsUsage: "[env-id]",
				Description: `If env-id is not given, it is derived from the current working directory.

The created environment is configured to have read-write access to the current
working directory with the exception of the .git folder, which is configured
read-only. This can be changed by passing the --no-cwd flag or by editing
the TOML config file of the created environment.
`,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "no-cwd",
						Aliases: []string{"nc"},
						Usage:   "Do not configure the environment to mount the current working directory",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					var envId string
					if cmd.NArg() > 1 {
						return cli.Exit("usage: drop init [env-id]", 1)
					}
					if cmd.NArg() == 0 {
						envId = defaultEnvId
					} else {
						envId = cmd.Args().First()
					}
					noCwd := cmd.Bool("no-cwd")
					if err := initEnv(envId, noCwd, homeDir, dropHome); err != nil {
						return fmt.Errorf("init environment: %v", err)
					}
					return nil
				},
			},
			{
				Name:  "run",
				Usage: "Run a command in a Drop environment",
				Description: `If -e is not given, env-id is derived from the current working directory.
If -c is not given, the config file is ENV_ID.toml in the Drop config directory.
The -m, -t, -T, -u, -U options are appended to options from the TOML config file.`,
				ArgsUsage:    "[command...]",
				StopOnNthArg: intPtr(1),
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:        "env",
						Aliases:     []string{"e"},
						Usage:       "Environment ID",
						Value:       defaultEnvId,
						Destination: &flags.EnvId,
					},
					&cli.StringFlag{
						Name:        "config",
						Aliases:     []string{"c"},
						Usage:       "Path to TOML config file",
						Destination: &flags.ConfigPath,
					},
					&cli.BoolFlag{
						Name:        "root",
						Aliases:     []string{"r"},
						Usage:       "Be root (uid 0) in the sandbox (doesn't grant any additional privileges to the sandboxed processes).",
						Destination: &flags.BeRoot,
					},
					&cli.StringFlag{
						Name:        "runtime",
						Usage:       "Sandboxing runtime: native or gvisor",
						Destination: &flags.Runtime,
					},
					&cli.StringSliceFlag{
						Name:        "mount",
						Aliases:     []string{"m"},
						Usage:       "Add a mount (format: source[:target][:rw])",
						Destination: &flags.Mounts,
					},
					&cli.StringFlag{
						Name:        "net",
						Aliases:     []string{"n"},
						Usage:       "Network mode: off or isolated",
						Destination: &flags.NetworkMode,
					},
					&cli.StringSliceFlag{
						Name:        "tcp-publish",
						Aliases:     []string{"t"},
						Usage:       "Publish a TCP port from the sandbox (format: [hostIP/]hostPort[:sandboxPort])",
						Destination: &flags.TcpPublishedPorts,
					},
					&cli.StringSliceFlag{
						Name:        "tcp-host",
						Aliases:     []string{"T"},
						Usage:       "Make a TCP port from the host available in the sandbox (format: hostPort[:sandboxPort])",
						Destination: &flags.TcpHostPorts,
					},
					&cli.StringSliceFlag{
						Name:        "udp-publish",
						Aliases:     []string{"u"},
						Usage:       "Publish a UDP port from the sandbox (format: [hostIP/]hostPort[:sandboxPort])",
						Destination: &flags.UdpPublishedPorts,
					},
					&cli.StringSliceFlag{
						Name:        "udp-host",
						Aliases:     []string{"U"},
						Usage:       "Make a UDP port from the host available in the sandbox (format: hostPort[:sandboxPort])",
						Destination: &flags.UdpHostPorts,
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					flags.Args = cmd.Args().Slice()

					if err := validateEnvId(flags.EnvId); err != nil {
						return err
					}
					return run(&flags, homeDir, dropHome)
				},
			},
			{
				Name:      "ls",
				Usage:     "List available Drop environments",
				ArgsUsage: " ",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.NArg() != 0 {
						return cli.Exit("usage: drop ls", 1)
					}
					if err := ls(dropHome); err != nil {
						return fmt.Errorf("list environments: %v", err)
					}
					return nil
				},
			},
			{
				Name:      "rm",
				Usage:     "Remove a Drop environment",
				ArgsUsage: "<env-id>",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.NArg() != 1 {
						return cli.Exit("usage: drop rm <env-id>", 1)
					}
					envId := cmd.Args().First()
					if err := jailfs.RmEnv(homeDir, dropHome, envId); err != nil {
						return fmt.Errorf("remove environment '%s': %v", envId, err)
					}
					return nil
				},
			},
			{
				Name:  "edit",
				Usage: "Edit a Drop config file",
				Description: `If env-id is not given, it is derived from the current working directory.
'base' can be passed instead of env-id to edit base.toml.
The editor is taken from $VISUAL, or from $EDITOR if $VISUAL is not set.`,
				ArgsUsage: "[env-id or base]",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					var configName string
					if cmd.NArg() > 1 {
						return cli.Exit("usage: drop edit [env-id or base]", 1)
					}
					if cmd.NArg() == 0 {
						configName = defaultEnvId
					} else {
						configName = cmd.Args().First()
					}
					if configName != "base" {
						if err := validateEnvId(configName); err != nil {
							return err
						}
					}
					return edit(configName, homeDir)
				},
			},
			{
				Name:        "update",
				Usage:       "Check if a new version of Drop is available",
				ArgsUsage:   " ",
				Description: "The --check flag is currently required. Automatic updating is not yet available.",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "check",
						Aliases: []string{"c"},
						Usage:   "Check for updates",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return update(version, cmd.Bool("check"))
				},
			},
		},
	}
}

func validateEnvId(envId string) error {
	if envId == "" {
		return fmt.Errorf("could not determine environment ID from current directory")
	}
	if !jailfs.IsEnvIdValid(envId) {
		return fmt.Errorf("invalid environment ID: %s", envId)
	}
	return nil
}

func intPtr(i int) *int {
	return &i
}
