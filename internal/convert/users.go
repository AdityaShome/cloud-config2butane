package convert

import (
	"fmt"
	"strings"

	butane "github.com/coreos/butane/base/v0_5"

	"github.com/AdityaShome/cloud-config2butane/internal/cloudconfig"
)

// defaultUserName is Flatcar's pre-existing default user. cloud-config's
// `name: default` is a placeholder for "the distro's default user", not a
// request to create a new one.
const defaultUserName = "core"

const sudoDropinPath = "/etc/sudoers.d/90-cloud-config2butane"

// canonicalUserName resolves "default" everywhere, so renamed and
// pre-rename references (e.g. groups[].members) still match.
func canonicalUserName(name string) string {
	if name == "default" {
		return defaultUserName
	}
	return name
}

// Users converts cloud-config users into Butane passwd users. Top-level
// ssh_authorized_keys are merged into the default user's keys, matching
// cloud-init's own semantics. Any user with a `sudo` rule gets a line in
// a generated sudoers.d drop-in, since Ignition has no native concept of
// sudo grants.
func Users(users []cloudconfig.User, topLevelSSHKeys []string) ([]butane.PasswdUser, *butane.File, []error) {
	var out []butane.PasswdUser
	var errs []error
	var sudoLines []string
	seen := map[string]bool{}

	for _, u := range users {
		if u.Name == "" {
			errs = append(errs, fmt.Errorf("user: missing name"))
			continue
		}
		name := canonicalUserName(u.Name)
		if seen[name] {
			errs = append(errs, fmt.Errorf("user %s: duplicate user name (note: \"default\" is an alias for %q)", name, defaultUserName))
			continue
		}
		seen[name] = true

		bu := butane.PasswdUser{Name: name}

		keys := u.SSHAuthorizedKeys
		if name == defaultUserName {
			keys = append(append([]string{}, topLevelSSHKeys...), keys...)
		}
		for _, k := range keys {
			bu.SSHAuthorizedKeys = append(bu.SSHAuthorizedKeys, butane.SSHAuthorizedKey(k))
		}

		seenGroup := map[string]bool{}
		for _, g := range u.Groups {
			if seenGroup[g] {
				continue
			}
			seenGroup[g] = true
			bu.Groups = append(bu.Groups, butane.Group(g))
		}

		if u.PrimaryGroup != "" {
			bu.PrimaryGroup = strPtr(u.PrimaryGroup)
		}
		if u.Shell != "" {
			bu.Shell = strPtr(u.Shell)
		}
		if u.Gecos != "" {
			bu.Gecos = strPtr(u.Gecos)
		}
		if u.NoCreateHome {
			bu.NoCreateHome = boolPtr(true)
		}

		if u.Passwd != "" {
			hash, err := passwordHash(name, u.Passwd, u.LockPasswd)
			if err != nil {
				errs = append(errs, err)
			} else {
				bu.PasswordHash = &hash
			}
		}

		for _, rule := range u.Sudo {
			rule = strings.TrimSpace(rule)
			if rule == "" {
				continue
			}
			sudoLines = append(sudoLines, name+" "+rule)
		}

		out = append(out, bu)
	}

	var sudoFile *butane.File
	if len(sudoLines) > 0 {
		content := strings.Join(sudoLines, "\n") + "\n"
		sudoFile = &butane.File{
			Path: sudoDropinPath,
			Mode: intPtr(0440),
			Contents: butane.Resource{
				Inline: &content,
			},
		}
	}

	return out, sudoFile, errs
}

// passwordHash validates that passwd is already a crypt(3) hash - Ignition
// has no way to hash a plaintext password at provisioning time - and
// applies cloud-config's lock_passwd convention: a password is locked by
// default, the same way `passwd -l` does it, by prefixing the hash with
// "!".
func passwordHash(user, passwd string, lock *bool) (string, error) {
	if !strings.HasPrefix(passwd, "$") {
		return "", fmt.Errorf("user %s: passwd must be a hashed password (e.g. $6$...), got a plaintext value", user)
	}
	if lock == nil || *lock {
		return "!" + passwd, nil
	}
	return passwd, nil
}

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }
func intPtr(i int) *int       { return &i }
