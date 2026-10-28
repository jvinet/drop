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

package firewall

import (
	"strings"
	"testing"
)

func TestParseAction(t *testing.T) {
	cases := []struct {
		in, want, wantErr string
	}{
		{"allow", "allow", ""},
		{"deny", "deny", ""},
		{"ask", "ask", ""},
		{"block", "", "invalid action \"block\""},
	}
	for _, c := range cases {
		got, err := parseAction(c.in)
		switch {
		case c.wantErr != "" && err == nil:
			t.Errorf("parseAction(%q): expected an error", c.in)
		case c.wantErr != "" && !strings.Contains(err.Error(), c.wantErr):
			t.Errorf("parseAction(%q): error %q, want %q", c.in, err, c.wantErr)
		case c.wantErr == "" && err != nil:
			t.Errorf("parseAction(%q): unexpected error %v", c.in, err)
		case c.wantErr == "" && got.String() != c.want:
			t.Errorf("parseAction(%q) = %s, want %s", c.in, got, c.want)
		}
	}
}
