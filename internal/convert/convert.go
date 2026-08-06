package convert

import (
	butane "github.com/coreos/butane/base/v0_5"
	butane1_1 "github.com/coreos/butane/config/flatcar/v1_1"

	"github.com/AdityaShome/cloud-config2butane/internal/cloudconfig"
)

const butaneVersion = "1.1.0"

// Convert assembles a full Butane config from a parsed cloud-config
// document. It runs every sub-converter and collects all of their
// errors in one pass - a cloud-config file with three problems should
// report three problems, not stop at the first.
func Convert(cfg *cloudconfig.Config) (*butane1_1.Config, []error) {
	var errs []error

	users, sudoFile, userErrs := Users(cfg.Users, cfg.SSHAuthorizedKeys)
	errs = append(errs, userErrs...)

	groups, membership := Groups(cfg.Groups)
	applyGroupMembership(users, membership)

	files, systemdUnits, fileErrs := Files(cfg.WriteFiles)
	errs = append(errs, fileErrs...)

	if sudoFile != nil {
		files = append(files, *sudoFile)
	}
	if hostnameFile := Hostname(cfg.Hostname, cfg.FQDN); hostnameFile != nil {
		files = append(files, *hostnameFile)
	}

	units := append([]butane.Unit{}, systemdUnits...)
	if scriptFile, unit := RunCmd(cfg.RunCmd); unit != nil {
		files = append(files, *scriptFile)
		units = append(units, *unit)
	}
	if scriptFile, unit := BootCmd(cfg.BootCmd); unit != nil {
		files = append(files, *scriptFile)
		units = append(units, *unit)
	}

	out := &butane1_1.Config{
		Config: butane.Config{
			Version: butaneVersion,
			Variant: "flatcar",
			Passwd: butane.Passwd{
				Users:  users,
				Groups: groups,
			},
			Storage: butane.Storage{
				Files: files,
			},
			Systemd: butane.Systemd{
				Units: units,
			},
		},
	}

	return out, errs
}

// applyGroupMembership adds each top-level group a user is listed as a
// member of onto that user's own Groups, since Butane has no separate
// "initial members" concept on passwd.groups. A member name with no
// matching users[] entry has nowhere to attach the group and is simply
// not applied - see Groups' doc comment.
func applyGroupMembership(users []butane.PasswdUser, membership map[string][]string) {
	for i := range users {
		for _, g := range membership[users[i].Name] {
			users[i].Groups = append(users[i].Groups, butane.Group(g))
		}
	}
}
