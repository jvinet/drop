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

import "strings"

// isValidDomain returns true if a domain name should be allowed in
// firewall rules and in outgoing DNS queries. A DNS query can carry
// exotic and generally invalid names, for example containing * or :.
// If such a domain was presented to the user in an interactive
// firewall prompt and then stored as a firewall rule on disk, it could
// result in an invalid rule or even a rule that grants additional
// access. To prevent such abuse, the firewall rejects queries with
// domains that do not pass conservative validation.
//
// The domain must be lowercase and without the trailing dot.
func isValidDomain(domain string) bool {
	if len(domain) == 0 || len(domain) > 253 {
		return false
	}
	labels := strings.Split(domain, ".")
	for _, l := range labels {
		if len(l) == 0 || len(l) > 63 || l[0] == '-' || l[len(l)-1] == '-' {
			return false
		}
		for i := 0; i < len(l); i++ {
			c := l[i]
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
				return false
			}
		}
	}
	// An all-numeric last label means a malformed IP, not a domain.
	tld := labels[len(labels)-1]
	return strings.Trim(tld, "0123456789") != ""
}
