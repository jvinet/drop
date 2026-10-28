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
	"fmt"
)

type action int

// The order matters: if the rules are equally specific, the one
// with the lowest action is used.
const (
	deny action = iota
	allow
	ask
)

func (a action) String() string {
	switch a {
	case allow:
		return "allow"
	case deny:
		return "deny"
	case ask:
		return "ask"
	default:
		return fmt.Sprintf("action(%d)", int(a))
	}
}

func parseAction(s string) (action, error) {
	switch s {
	case "allow":
		return allow, nil
	case "deny":
		return deny, nil
	case "ask":
		return ask, nil
	}
	return deny, fmt.Errorf("invalid action %q, expected allow, deny or ask", s)
}
