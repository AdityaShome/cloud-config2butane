package convert

import (
	"fmt"

	butane "github.com/coreos/butane/base/v0_5"

	"github.com/AdityaShome/cloud-config2butane/internal/cloudconfig"
)

// Groups converts cloud-config top-level groups into Butane passwd
// groups. Butane's PasswdGroup has no "initial members" field - group
// membership can only be expressed on the member's own
// passwd.users[].groups - so this also returns a username -> group-names
// map for Convert to merge into any matching users[] entry. Member names
// are canonicalized (see canonicalUserName) so "default" still matches
// after rename. A member not in users[] has nothing to attach to and is
// dropped, which is documented as a limitation. A group name declared
// more than once is rejected rather than silently keeping one of two
// possibly-conflicting member lists.
func Groups(groups cloudconfig.GroupList) ([]butane.PasswdGroup, map[string][]string, []error) {
	var out []butane.PasswdGroup
	membership := map[string][]string{}
	seen := map[string]bool{}
	var errs []error

	for _, g := range groups {
		if seen[g.Name] {
			errs = append(errs, fmt.Errorf("group %s: defined more than once", g.Name))
			continue
		}
		seen[g.Name] = true

		out = append(out, butane.PasswdGroup{Name: g.Name})
		for _, member := range g.Members {
			name := canonicalUserName(member)
			membership[name] = append(membership[name], g.Name)
		}
	}

	return out, membership, errs
}
