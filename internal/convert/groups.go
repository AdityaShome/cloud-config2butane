package convert

import (
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
// dropped, which is documented as a limitation.
func Groups(groups cloudconfig.GroupList) ([]butane.PasswdGroup, map[string][]string) {
	var out []butane.PasswdGroup
	membership := map[string][]string{}

	for _, g := range groups {
		out = append(out, butane.PasswdGroup{Name: g.Name})
		for _, member := range g.Members {
			name := canonicalUserName(member)
			membership[name] = append(membership[name], g.Name)
		}
	}

	return out, membership
}
