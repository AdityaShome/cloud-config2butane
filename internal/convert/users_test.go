package convert

import (
	"strings"
	"testing"

	"github.com/AdityaShome/cloud-config2butane/internal/cloudconfig"
)

func TestUsersBasic(t *testing.T) {
	users := []cloudconfig.User{
		{Name: "alice", Shell: "/bin/bash", Gecos: "Alice"},
	}
	out, sudoFile, errs := Users(users, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if sudoFile != nil {
		t.Fatalf("expected no sudo file, got %+v", sudoFile)
	}
	if len(out) != 1 || out[0].Name != "alice" {
		t.Fatalf("got %+v", out)
	}
	if out[0].Shell == nil || *out[0].Shell != "/bin/bash" {
		t.Errorf("shell: got %v", out[0].Shell)
	}
	if out[0].Gecos == nil || *out[0].Gecos != "Alice" {
		t.Errorf("gecos: got %v", out[0].Gecos)
	}
	if out[0].PrimaryGroup != nil || len(out[0].Groups) != 0 {
		t.Errorf("expected no groups set, got primary=%v groups=%v", out[0].PrimaryGroup, out[0].Groups)
	}
}

func TestUsersPrimaryGroup(t *testing.T) {
	users := []cloudconfig.User{{Name: "alice", PrimaryGroup: "wheel"}}
	out, _, _ := Users(users, nil)
	if out[0].PrimaryGroup == nil || *out[0].PrimaryGroup != "wheel" {
		t.Errorf("got primary group %v, want wheel", out[0].PrimaryGroup)
	}
}

func TestUsersMissingOptionalFields(t *testing.T) {
	users := []cloudconfig.User{{Name: "bob"}}
	out, _, errs := Users(users, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	u := out[0]
	if u.Shell != nil || u.Gecos != nil || u.PrimaryGroup != nil || u.PasswordHash != nil || u.NoCreateHome != nil {
		t.Errorf("expected all optional fields nil, got %+v", u)
	}
}

func TestUsersMissingName(t *testing.T) {
	users := []cloudconfig.User{{Shell: "/bin/bash"}}
	out, _, errs := Users(users, nil)
	if len(out) != 0 {
		t.Errorf("expected nameless user to be skipped, got %+v", out)
	}
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %v", errs)
	}
}

func TestUsersGroups(t *testing.T) {
	users := []cloudconfig.User{
		{Name: "alice", Groups: cloudconfig.CommaList{"wheel", "docker"}},
	}
	out, _, _ := Users(users, nil)
	if len(out[0].Groups) != 2 || string(out[0].Groups[0]) != "wheel" || string(out[0].Groups[1]) != "docker" {
		t.Errorf("got groups %v", out[0].Groups)
	}
}

func TestUsersGroupsDedupesDuplicateEntry(t *testing.T) {
	users := []cloudconfig.User{
		{Name: "alice", Groups: cloudconfig.CommaList{"wheel", "wheel"}},
	}
	out, _, _ := Users(users, nil)
	if len(out[0].Groups) != 1 || string(out[0].Groups[0]) != "wheel" {
		t.Errorf("got groups %v, want a single wheel entry", out[0].Groups)
	}
}

func TestUsersPasswdHashedAndLocked(t *testing.T) {
	users := []cloudconfig.User{
		{Name: "alice", Passwd: "$6$rounds=4096$salt$hash"},
	}
	out, _, errs := Users(users, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if out[0].PasswordHash == nil || *out[0].PasswordHash != "!$6$rounds=4096$salt$hash" {
		t.Errorf("got password hash %v, want locked (! prefix)", out[0].PasswordHash)
	}
}

func TestUsersPasswdUnlockedWhenLockPasswdFalse(t *testing.T) {
	no := false
	users := []cloudconfig.User{
		{Name: "alice", Passwd: "$6$rounds=4096$salt$hash", LockPasswd: &no},
	}
	out, _, errs := Users(users, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if out[0].PasswordHash == nil || *out[0].PasswordHash != "$6$rounds=4096$salt$hash" {
		t.Errorf("got password hash %v, want unlocked (no ! prefix)", out[0].PasswordHash)
	}
}

func TestUsersPasswdPlaintextRejected(t *testing.T) {
	users := []cloudconfig.User{
		{Name: "alice", Passwd: "hunter2"},
	}
	out, _, errs := Users(users, nil)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "plaintext") {
		t.Fatalf("expected a plaintext-passwd error, got %v", errs)
	}
	if out[0].PasswordHash != nil {
		t.Errorf("expected no password hash to be set, got %v", out[0].PasswordHash)
	}
	if out[0].Name != "alice" {
		t.Errorf("expected the rest of the user to still convert, got %+v", out[0])
	}
}

func TestUsersDefaultUserRenamedAndMergesTopLevelSSHKeys(t *testing.T) {
	users := []cloudconfig.User{
		{Name: "default", SSHAuthorizedKeys: []string{"ssh-ed25519 per-user"}},
	}
	out, _, errs := Users(users, []string{"ssh-ed25519 top-level"})
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if out[0].Name != "core" {
		t.Fatalf("expected default user renamed to core, got %q", out[0].Name)
	}
	got := make([]string, len(out[0].SSHAuthorizedKeys))
	for i, k := range out[0].SSHAuthorizedKeys {
		got[i] = string(k)
	}
	want := []string{"ssh-ed25519 top-level", "ssh-ed25519 per-user"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got keys %v, want %v", got, want)
	}
}

func TestUsersTopLevelSSHKeysDoNotLeakToOtherUsers(t *testing.T) {
	users := []cloudconfig.User{
		{Name: "alice", SSHAuthorizedKeys: []string{"ssh-ed25519 alice"}},
	}
	out, _, errs := Users(users, []string{"ssh-ed25519 top-level"})
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(out[0].SSHAuthorizedKeys) != 1 || string(out[0].SSHAuthorizedKeys[0]) != "ssh-ed25519 alice" {
		t.Errorf("expected top-level keys to only apply to the default user, got %v", out[0].SSHAuthorizedKeys)
	}
}

func TestUsersSudoStringAndListProduceSameLine(t *testing.T) {
	stringForm := []cloudconfig.User{
		{Name: "alice", Sudo: cloudconfig.SudoRules{"ALL=(ALL) NOPASSWD:ALL"}},
	}
	_, fileA, _ := Users(stringForm, nil)
	if fileA == nil || fileA.Contents.Inline == nil {
		t.Fatal("expected a sudo file with inline content")
	}
	if *fileA.Contents.Inline != "alice ALL=(ALL) NOPASSWD:ALL\n" {
		t.Errorf("got sudo file content %q", *fileA.Contents.Inline)
	}
	if fileA.Path != sudoDropinPath {
		t.Errorf("got path %q, want %q", fileA.Path, sudoDropinPath)
	}
	if fileA.Mode == nil || *fileA.Mode != 0440 {
		t.Errorf("got mode %v, want 0440", fileA.Mode)
	}
}

func TestUsersSudoMultipleUsersCombinedInOneFile(t *testing.T) {
	users := []cloudconfig.User{
		{Name: "alice", Sudo: cloudconfig.SudoRules{"ALL=(ALL) NOPASSWD:ALL"}},
		{Name: "bob", Sudo: cloudconfig.SudoRules{"ALL=(ALL) ALL"}},
	}
	_, sudoFile, _ := Users(users, nil)
	want := "alice ALL=(ALL) NOPASSWD:ALL\nbob ALL=(ALL) ALL\n"
	if sudoFile == nil || sudoFile.Contents.Inline == nil || *sudoFile.Contents.Inline != want {
		t.Fatalf("got %+v, want content %q", sudoFile, want)
	}
}

func TestUsersNoSudoMeansNoSudoFile(t *testing.T) {
	users := []cloudconfig.User{{Name: "alice"}}
	_, sudoFile, _ := Users(users, nil)
	if sudoFile != nil {
		t.Errorf("expected no sudo file, got %+v", sudoFile)
	}
}

func TestUsersSudoBlankRuleIgnored(t *testing.T) {
	users := []cloudconfig.User{
		{Name: "alice", Sudo: cloudconfig.SudoRules{"  ", "ALL=(ALL) NOPASSWD:ALL"}},
	}
	_, sudoFile, _ := Users(users, nil)
	want := "alice ALL=(ALL) NOPASSWD:ALL\n"
	if sudoFile == nil || *sudoFile.Contents.Inline != want {
		t.Fatalf("got %+v, want content %q", sudoFile, want)
	}
}

func TestUsersDuplicateNameRejected(t *testing.T) {
	users := []cloudconfig.User{
		{Name: "alice", SSHAuthorizedKeys: []string{"ssh-ed25519 first"}},
		{Name: "alice", Sudo: cloudconfig.SudoRules{"ALL=(ALL) NOPASSWD:ALL"}},
	}
	out, _, errs := Users(users, nil)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "duplicate") {
		t.Fatalf("expected a duplicate-name error, got %v", errs)
	}
	if len(out) != 1 || len(out[0].SSHAuthorizedKeys) != 1 {
		t.Fatalf("expected only the first entry to be kept, got %+v", out)
	}
}

func TestUsersDuplicateNameViaDefaultAliasRejected(t *testing.T) {
	users := []cloudconfig.User{
		{Name: "default", SSHAuthorizedKeys: []string{"ssh-ed25519 default-key"}},
		{Name: "core", Sudo: cloudconfig.SudoRules{"ALL=(ALL) NOPASSWD:ALL"}},
	}
	out, _, errs := Users(users, nil)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "duplicate") {
		t.Fatalf("expected a duplicate-name error, got %v", errs)
	}
	if len(out) != 1 || len(out[0].SSHAuthorizedKeys) != 1 {
		t.Fatalf("expected only the \"default\" entry to be kept, got %+v", out)
	}
}

func TestUsersNoCreateHome(t *testing.T) {
	users := []cloudconfig.User{{Name: "alice", NoCreateHome: true}}
	out, _, _ := Users(users, nil)
	if out[0].NoCreateHome == nil || !*out[0].NoCreateHome {
		t.Errorf("got NoCreateHome %v, want true", out[0].NoCreateHome)
	}
}
