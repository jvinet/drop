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

func TestIsValidDomain(t *testing.T) {
	label63 := strings.Repeat("a", 63)
	label64 := strings.Repeat("a", 64)
	// Three 63 character labels and dots take 192 characters.
	name253 := strings.Join([]string{label63, label63, label63, strings.Repeat("b", 61)}, ".")
	name254 := strings.Join([]string{label63, label63, label63, strings.Repeat("b", 62)}, ".")

	cases := []struct {
		domain string
		want   bool
	}{
		{"example.com", true},
		{"a.b.example.com", true},
		{"nas", true},
		{"my-host.example.com", true},
		{"_dmarc.example.com", true},
		{"xn--bcher-kva.de", true},
		{"0.example.com", true},
		{"example.c0m", true},
		{label63 + ".com", true},
		{name253, true},

		{"", false},
		{".", false},
		{"example.com.", false},
		{".example.com", false},
		{"a..b", false},
		{"Example.com", false},
		{"*.com", false},
		{"x.com:443", false},
		{"a,b.com", false},
		{"a/b.com", false},
		{"a b.com", false},
		{"a#b.com", false},
		{"bücher.de", false},
		{"-a.com", false},
		{"a-.com", false},
		{label64 + ".com", false},
		{name254, false},
		{"10.0.0.256", false},
		{"example.123", false},
	}
	for _, c := range cases {
		if got := isValidDomain(c.domain); got != c.want {
			t.Errorf("isValidDomain(%q) = %v, want %v", c.domain, got, c.want)
		}
	}
}
