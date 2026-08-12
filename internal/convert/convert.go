package convert

import (
	"fmt"

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

	files, dupErrs := dedupeFilePaths(files)
	errs = append(errs, dupErrs...)

	units, dupUnitErrs := dedupeUnitNames(units)
	errs = append(errs, dupUnitErrs...)

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

// dedupeFilePaths keeps the first storage.files entry per path and
// errors on any later one that reuses it, including our own generated
// files.
func dedupeFilePaths(files []butane.File) ([]butane.File, []error) {
	seen := map[string]bool{}
	var out []butane.File
	var errs []error
	for _, f := range files {
		if seen[f.Path] {
			errs = append(errs, fmt.Errorf("write_files: path %s is used by more than one file", f.Path))
			continue
		}
		seen[f.Path] = true
		out = append(out, f)
	}
	return out, errs
}

// dedupeUnitNames keeps the first systemd unit per name and errors on
// any later one that reuses it, including our own generated units.
func dedupeUnitNames(units []butane.Unit) ([]butane.Unit, []error) {
	seen := map[string]bool{}
	var out []butane.Unit
	var errs []error
	for _, u := range units {
		if seen[u.Name] {
			errs = append(errs, fmt.Errorf("write_files: systemd unit %s is defined by more than one file", u.Name))
			continue
		}
		seen[u.Name] = true
		out = append(out, u)
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
