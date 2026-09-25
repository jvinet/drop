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

// 'drop edit ...' command handling

package command

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/wrr/drop/internal/jailfs"
)

// Edit opens a Drop config file in the user's editor ('drop
// edit envid' command). 'base' can be passed instead of an
// environment id to open the config file shared by all environments.
//
// $VISUAL takes precedence over $EDITOR. If neither is set, the
// config path is just printed.
func Edit(configName, homeDir string) error {
	var path string
	isBase := (configName == "base")
	if isBase {
		path = jailfs.BaseConfigPath(homeDir)
	} else {
		path = jailfs.EnvConfigPath(homeDir, configName)
	}
	if _, err := os.Stat(path); err != nil {
		hdr := "can't edit config:"
		if os.IsNotExist(err) {
			if isBase {
				return fmt.Errorf("%s base.toml doesn't exist, run 'drop init [env-id]' to create it", hdr)
			}
			return fmt.Errorf("%s environment %q doesn't exist, run 'drop init %v' to create it", hdr, configName, configName)
		}
		return fmt.Errorf("%s %v", hdr, err)
	}
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		fmt.Printf("Neither $VISUAL nor $EDITOR is set. Edit the config at: %s\n", path)
		return nil
	}
	cmd := exec.Command(parts[0], append(parts[1:], path)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
