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

// Package updater checks for new versions of Drop.
package updater

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const releaseURL = "https://api.github.com/repos/wrr/drop/releases/latest"

const httpTimeout = 10 * time.Second

// CheckForUpdate checks if a newer version of Drop is available.
// Returns the new version string if an update is available, empty
// string otherwise.
func CheckForUpdate(currentVersion string) (string, error) {
	return doCheckForUpdate(releaseURL, currentVersion)
}

// Released binaries have a version format MAJOR.MINOR.PATCH. Anything
// else is a development build.
var releaseVersion = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// A separate function to allow to change url in tests.
func doCheckForUpdate(url, currentVersion string) (string, error) {
	if !releaseVersion.MatchString(currentVersion) {
		return "", fmt.Errorf("development build")
	}

	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected HTTP status %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("invalid response: %v", err)
	}

	// Tags have 'v' prefix, version strings do not.
	latestVersion := strings.TrimPrefix(release.TagName, "v")

	if latestVersion != currentVersion {
		return latestVersion, nil
	}
	return "", nil
}
