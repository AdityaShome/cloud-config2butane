package convert

import (
	"strings"
	"testing"

	"github.com/AdityaShome/cloud-config2butane/internal/cloudconfig"
)

func TestConvertAggregatesErrorsFromMultipleConverters(t *testing.T) {
	cfg := &cloudconfig.Config{
		Users: []cloudconfig.User{
			{Passwd: "plaintext"}, // missing name AND plaintext passwd
		},
		WriteFiles: []cloudconfig.File{
			{Path: "/etc/x", Encoding: "rot13"},
		},
	}
	_, errs := Convert(cfg)
	if len(errs) < 2 {
		t.Fatalf("expected errors from both the users and files converters, got %v", errs)
	}
}

func TestConvertGroupMembershipMergedIntoMatchingUser(t *testing.T) {
	cfg := &cloudconfig.Config{
		Users: []cloudconfig.User{{Name: "alice"}},
		Groups: cloudconfig.GroupList{
			{Name: "admingroup", Members: []string{"alice"}},
		},
	}
	out, errs := Convert(cfg)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(out.Passwd.Users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(out.Passwd.Users))
	}
	u := out.Passwd.Users[0]
	if len(u.Groups) != 1 || string(u.Groups[0]) != "admingroup" {
		t.Errorf("got groups %v, want [admingroup]", u.Groups)
	}
	if len(out.Passwd.Groups) != 1 || out.Passwd.Groups[0].Name != "admingroup" {
		t.Errorf("got groups %v", out.Passwd.Groups)
	}
}

func TestConvertGroupMembershipForDefaultUserResolvesAfterRename(t *testing.T) {
	cfg := &cloudconfig.Config{
		Users: []cloudconfig.User{{Name: "default"}},
		Groups: cloudconfig.GroupList{
			{Name: "wheel", Members: []string{"default"}},
		},
	}
	out, errs := Convert(cfg)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(out.Passwd.Users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(out.Passwd.Users))
	}
	u := out.Passwd.Users[0]
	if u.Name != defaultUserName {
		t.Fatalf("expected user renamed to %q, got %q", defaultUserName, u.Name)
	}
	if len(u.Groups) != 1 || string(u.Groups[0]) != "wheel" {
		t.Errorf("expected %q's group membership to survive the default->%s rename, got %v", "default", defaultUserName, u.Groups)
	}
}

func TestConvertGroupMembershipForUnknownUserIsDropped(t *testing.T) {
	cfg := &cloudconfig.Config{
		Groups: cloudconfig.GroupList{
			{Name: "admingroup", Members: []string{"root", "sys"}},
		},
	}
	out, errs := Convert(cfg)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(out.Passwd.Users) != 0 {
		t.Errorf("expected no users to be created out of thin air, got %v", out.Passwd.Users)
	}
	if len(out.Passwd.Groups) != 1 {
		t.Errorf("expected the group itself to still be created, got %v", out.Passwd.Groups)
	}
}

func TestConvertDuplicateFilePathRejected(t *testing.T) {
	cfg := &cloudconfig.Config{
		Hostname: "node1",
		WriteFiles: []cloudconfig.File{
			{Path: "/etc/hostname", Content: "user-supplied\n"},
		},
	}
	out, errs := Convert(cfg)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "/etc/hostname") {
		t.Fatalf("expected a duplicate-path error mentioning /etc/hostname, got %v", errs)
	}
	count := 0
	for _, f := range out.Storage.Files {
		if f.Path == "/etc/hostname" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected only 1 file at /etc/hostname in the returned config, got %d", count)
	}
}

func TestConvertSudoFileMergedIntoStorageFiles(t *testing.T) {
	cfg := &cloudconfig.Config{
		Users: []cloudconfig.User{
			{Name: "alice", Sudo: cloudconfig.SudoRules{"ALL=(ALL) NOPASSWD:ALL"}},
		},
	}
	out, errs := Convert(cfg)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	found := false
	for _, f := range out.Storage.Files {
		if f.Path == sudoDropinPath {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a sudoers.d file in storage.files, got %+v", out.Storage.Files)
	}
}

func TestConvertRunCmdAndBootCmdBothProduceUnits(t *testing.T) {
	cfg := &cloudconfig.Config{
		RunCmd:  cloudconfig.CommandList{{Line: "echo run"}},
		BootCmd: cloudconfig.CommandList{{Line: "echo boot"}},
	}
	out, errs := Convert(cfg)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(out.Systemd.Units) != 2 {
		t.Fatalf("expected 2 units, got %d: %+v", len(out.Systemd.Units), out.Systemd.Units)
	}
	if len(out.Storage.Files) != 2 {
		t.Fatalf("expected 2 generated scripts, got %d", len(out.Storage.Files))
	}
}

func TestConvertHostnameMergedIntoStorageFiles(t *testing.T) {
	cfg := &cloudconfig.Config{Hostname: "node1"}
	out, errs := Convert(cfg)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(out.Storage.Files) != 1 || out.Storage.Files[0].Path != hostnamePath {
		t.Errorf("got %+v", out.Storage.Files)
	}
}

func TestConvertSystemdPassthroughUnitsIncluded(t *testing.T) {
	cfg := &cloudconfig.Config{
		WriteFiles: []cloudconfig.File{
			{Path: "/etc/systemd/system/demo.service", Content: "[Service]\nExecStart=/bin/true\n"},
		},
	}
	out, errs := Convert(cfg)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(out.Systemd.Units) != 1 || out.Systemd.Units[0].Name != "demo.service" {
		t.Errorf("got units %+v", out.Systemd.Units)
	}
	if len(out.Storage.Files) != 0 {
		t.Errorf("expected the unit not to also appear as a generic file, got %+v", out.Storage.Files)
	}
}

func TestConvertBasicVersionAndVariant(t *testing.T) {
	out, errs := Convert(&cloudconfig.Config{})
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if out.Version != "1.1.0" || out.Variant != "flatcar" {
		t.Errorf("got version=%q variant=%q", out.Version, out.Variant)
	}
}
