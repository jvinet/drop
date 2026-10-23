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

// 'drop update ...' command handling

package cli

import (
	"fmt"

	"github.com/wrr/drop/internal/updater"
)

func update(version string, checkOnly bool) error {
	if !checkOnly {
		return fmt.Errorf("automatic updating not yet available, you can check for updates with 'drop update --check'")
	}
	newVersion, err := updater.CheckForUpdate(version)
	if err != nil {
		return fmt.Errorf("check for updates: %v", err)
	}
	if newVersion != "" {
		fmt.Printf("Drop %s is available, your installed version is %s\n", newVersion, version)
		fmt.Printf("Download: https://github.com/wrr/drop/releases/latest\n")
	} else {
		fmt.Printf("Drop is up to date (version %s)\n", version)
	}
	return nil
}
