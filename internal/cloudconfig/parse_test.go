package cloudconfig

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func mustParse(t *testing.T, doc string) *Config {
	t.Helper()
	cfg, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse() returned unexpected error: %v", err)
	}
	return cfg
}

func TestParseHeader(t *testing.T) {
	withHeader := "#cloud-config\nhostname: node1\n"
	withoutHeader := "hostname: node1\n"

	got := mustParse(t, withHeader)
	want := mustParse(t, withoutHeader)
	if got.Hostname != want.Hostname || got.Hostname != "node1" {
		t.Fatalf("header stripping changed the result: with=%q without=%q", got.Hostname, want.Hostname)
	}
}

func TestParseHeaderPreservesLineNumbers(t *testing.T) {
	// hostname is on line 3 in both cases; a malformed users block below it
	// should report the same line whether or not the header is present.
	withHeader := "#cloud-config\nhostname: node1\nusers: not-a-list\n"
	withoutHeader := "\nhostname: node1\nusers: not-a-list\n"

	_, err1 := Parse([]byte(withHeader))
	_, err2 := Parse([]byte(withoutHeader))

	var pe1, pe2 *ParseError
	if !errors.As(err1, &pe1) || !errors.As(err2, &pe2) {
		t.Fatalf("expected ParseError, got %v / %v", err1, err2)
	}
	if pe1.Line != pe2.Line || pe1.Line != 3 {
		t.Fatalf("line numbers diverged: with=%d without=%d, want both 3", pe1.Line, pe2.Line)
	}
}

func TestParseUserGroupsBothForms(t *testing.T) {
	commaForm := `
users:
  - name: alice
    groups: wheel, docker
`
	listForm := `
users:
  - name: alice
    groups: [wheel, docker]
`
	want := CommaList{"wheel", "docker"}

	for _, doc := range []string{commaForm, listForm} {
		cfg := mustParse(t, doc)
		if len(cfg.Users) != 1 {
			t.Fatalf("expected 1 user, got %d", len(cfg.Users))
		}
		if !reflect.DeepEqual(cfg.Users[0].Groups, want) {
			t.Errorf("doc %q: got groups %v, want %v", doc, cfg.Users[0].Groups, want)
		}
	}
}

func TestParseUserSudoBothForms(t *testing.T) {
	stringForm := `
users:
  - name: alice
    sudo: ALL=(ALL) NOPASSWD:ALL
`
	listForm := `
users:
  - name: alice
    sudo:
      - ALL=(ALL) NOPASSWD:ALL
`
	want := SudoRules{"ALL=(ALL) NOPASSWD:ALL"}

	for _, doc := range []string{stringForm, listForm} {
		cfg := mustParse(t, doc)
		if !reflect.DeepEqual(cfg.Users[0].Sudo, want) {
			t.Errorf("doc %q: got sudo %v, want %v", doc, cfg.Users[0].Sudo, want)
		}
	}
}

func TestParseUserSudoFalseMeansNoSudo(t *testing.T) {
	doc := `
users:
  - name: alice
    sudo: false
`
	cfg := mustParse(t, doc)
	if cfg.Users[0].Sudo != nil {
		t.Errorf("got sudo %v, want nil (no sudo access)", cfg.Users[0].Sudo)
	}
}

func TestParseUserSudoTrueRejected(t *testing.T) {
	doc := `
users:
  - name: alice
    sudo: true
`
	_, err := Parse([]byte(doc))
	if err == nil {
		t.Fatal("expected an error for sudo: true, got nil")
	}
	if !strings.Contains(err.Error(), "sudo") {
		t.Errorf("got %v, want an error mentioning sudo", err)
	}
}

func TestParseRunCmdBothForms(t *testing.T) {
	doc := `
runcmd:
  - systemctl restart sshd
  - [mkdir, -p, /opt/data]
`
	cfg := mustParse(t, doc)
	if len(cfg.RunCmd) != 2 {
		t.Fatalf("expected 2 runcmd entries, got %d", len(cfg.RunCmd))
	}
	if cfg.RunCmd[0].IsArgv() || cfg.RunCmd[0].Line != "systemctl restart sshd" {
		t.Errorf("entry 0: got %+v", cfg.RunCmd[0])
	}
	if !cfg.RunCmd[1].IsArgv() || !reflect.DeepEqual(cfg.RunCmd[1].Argv, []string{"mkdir", "-p", "/opt/data"}) {
		t.Errorf("entry 1: got %+v", cfg.RunCmd[1])
	}
}

func TestParseBootCmdSeparateFromRunCmd(t *testing.T) {
	doc := `
runcmd:
  - echo once
bootcmd:
  - echo every-boot
`
	cfg := mustParse(t, doc)
	if len(cfg.RunCmd) != 1 || cfg.RunCmd[0].Line != "echo once" {
		t.Errorf("runcmd: got %+v", cfg.RunCmd)
	}
	if len(cfg.BootCmd) != 1 || cfg.BootCmd[0].Line != "echo every-boot" {
		t.Errorf("bootcmd: got %+v", cfg.BootCmd)
	}
}

func TestParseTopLevelGroupsBothForms(t *testing.T) {
	doc := `
groups:
  - admingroup: [root, sys]
  - cloud-users
`
	cfg := mustParse(t, doc)
	want := GroupList{
		{Name: "admingroup", Members: []string{"root", "sys"}},
		{Name: "cloud-users"},
	}
	if !reflect.DeepEqual(cfg.Groups, want) {
		t.Errorf("got %+v, want %+v", cfg.Groups, want)
	}
}

func TestParsePermissionsOctalStringAndInt(t *testing.T) {
	quoted := `
write_files:
  - path: /etc/foo
    permissions: "0644"
`
	bare := `
write_files:
  - path: /etc/foo
    permissions: 0644
`
	noLeadingZero := `
write_files:
  - path: /etc/foo
    permissions: "644"
`

	for _, doc := range []string{quoted, bare, noLeadingZero} {
		cfg := mustParse(t, doc)
		perm := cfg.WriteFiles[0].Permissions
		if !perm.IsSet() || perm.Value != 0644 {
			t.Errorf("doc %q: got mode %o (set=%v), want 0644", doc, perm.Value, perm.IsSet())
		}
	}
}

func TestParseInvalidPermissions(t *testing.T) {
	doc := `
write_files:
  - path: /etc/foo
    permissions: "not-octal"
`
	_, err := Parse([]byte(doc))
	if err == nil {
		t.Fatal("expected an error for non-octal permissions, got nil")
	}
}

func TestParseMissingHeaderStillParses(t *testing.T) {
	doc := "hostname: node1\n"
	cfg := mustParse(t, doc)
	if cfg.Hostname != "node1" {
		t.Errorf("got hostname %q, want node1", cfg.Hostname)
	}
}

func TestParseMalformedYAML(t *testing.T) {
	doc := "users:\n  - name: alice\n  bad indent: [\n"
	_, err := Parse([]byte(doc))
	if err == nil {
		t.Fatal("expected an error for malformed YAML, got nil")
	}
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected a *ParseError, got %T: %v", err, err)
	}
	if pe.Line == 0 {
		t.Errorf("expected a line number, got 0")
	}
}

func TestParseUnsupportedTopLevelField(t *testing.T) {
	doc := `
hostname: node1
packages:
  - htop
`
	_, err := Parse([]byte(doc))
	if err == nil {
		t.Fatal("expected an error for unsupported field, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported field: packages") {
		t.Errorf("got %v, want it to mention unsupported field: packages", err)
	}
}

func TestParseUnsupportedFieldsAreAllReported(t *testing.T) {
	doc := `
packages:
  - htop
power_state:
  mode: reboot
`
	_, err := Parse([]byte(doc))
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	for _, want := range []string{"unsupported field: packages", "unsupported field: power_state"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("got %v, want it to also mention %q", err, want)
		}
	}
}

func TestParseEmptyDocument(t *testing.T) {
	for _, doc := range []string{"", "#cloud-config\n", "#cloud-config"} {
		cfg := mustParse(t, doc)
		if !reflect.DeepEqual(*cfg, Config{}) {
			t.Errorf("doc %q: got non-empty config %+v", doc, cfg)
		}
	}
}

func TestParseSSHKeysTopLevelAndPerUser(t *testing.T) {
	doc := `
ssh_authorized_keys:
  - ssh-ed25519 AAAAtop
users:
  - name: alice
    ssh_authorized_keys:
      - ssh-ed25519 AAAAalice
`
	cfg := mustParse(t, doc)
	if !reflect.DeepEqual(cfg.SSHAuthorizedKeys, []string{"ssh-ed25519 AAAAtop"}) {
		t.Errorf("got top-level keys %v", cfg.SSHAuthorizedKeys)
	}
	if !reflect.DeepEqual(cfg.Users[0].SSHAuthorizedKeys, []string{"ssh-ed25519 AAAAalice"}) {
		t.Errorf("got per-user keys %v", cfg.Users[0].SSHAuthorizedKeys)
	}
}

func TestParseFQDNWithoutHostname(t *testing.T) {
	doc := "fqdn: node1.example.com\n"
	cfg := mustParse(t, doc)
	if cfg.FQDN != "node1.example.com" || cfg.Hostname != "" {
		t.Errorf("got fqdn=%q hostname=%q", cfg.FQDN, cfg.Hostname)
	}
}

func TestParseGroupsNotAList(t *testing.T) {
	_, err := Parse([]byte("groups: not-a-list\n"))
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestParseGroupEntryWrongKind(t *testing.T) {
	_, err := Parse([]byte("groups:\n  - [a, b]\n"))
	if err == nil {
		t.Fatal("expected an error for a list-shaped group entry, got nil")
	}
}

func TestParseGroupMembersInvalid(t *testing.T) {
	_, err := Parse([]byte("groups:\n  - admingroup: not-a-list\n"))
	if err == nil {
		t.Fatal("expected an error for non-list group members, got nil")
	}
}

func TestParseRunCmdNotAList(t *testing.T) {
	_, err := Parse([]byte("runcmd: not-a-list\n"))
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestParseRunCmdArgvDecodeError(t *testing.T) {
	_, err := Parse([]byte("runcmd:\n  - [1, {a: b}]\n"))
	if err == nil {
		t.Fatal("expected an error for a non-string argv element, got nil")
	}
}

func TestParseRunCmdEntryWrongKind(t *testing.T) {
	_, err := Parse([]byte("runcmd:\n  - {a: b}\n"))
	if err == nil {
		t.Fatal("expected an error for a map-shaped runcmd entry, got nil")
	}
}

func TestParseUserGroupsSequenceDecodeError(t *testing.T) {
	doc := `
users:
  - name: alice
    groups: [[a, b]]
`
	_, err := Parse([]byte(doc))
	if err == nil {
		t.Fatal("expected an error for a nested-list group entry, got nil")
	}
}

func TestParseUserGroupsWrongKind(t *testing.T) {
	doc := `
users:
  - name: alice
    groups: {a: b}
`
	_, err := Parse([]byte(doc))
	if err == nil {
		t.Fatal("expected an error for a map-shaped groups field, got nil")
	}
}

func TestParseUserSudoSequenceDecodeError(t *testing.T) {
	doc := `
users:
  - name: alice
    sudo: [[a, b]]
`
	_, err := Parse([]byte(doc))
	if err == nil {
		t.Fatal("expected an error for a nested-list sudo entry, got nil")
	}
}

func TestParseUserSudoWrongKind(t *testing.T) {
	doc := `
users:
  - name: alice
    sudo: {a: b}
`
	_, err := Parse([]byte(doc))
	if err == nil {
		t.Fatal("expected an error for a map-shaped sudo field, got nil")
	}
}

func TestParsePermissionsWrongKind(t *testing.T) {
	doc := `
write_files:
  - path: /etc/foo
    permissions: [1, 2]
`
	_, err := Parse([]byte(doc))
	if err == nil {
		t.Fatal("expected an error for a list-shaped permissions field, got nil")
	}
}

func TestParsePermissionsEmptyString(t *testing.T) {
	doc := `
write_files:
  - path: /etc/foo
    permissions: ""
`
	cfg := mustParse(t, doc)
	if cfg.WriteFiles[0].Permissions.IsSet() {
		t.Errorf("expected permissions to be unset for an empty string, got %v", cfg.WriteFiles[0].Permissions)
	}
}

func TestParseUserGroupsEmptyString(t *testing.T) {
	doc := `
users:
  - name: alice
    groups: ""
`
	cfg := mustParse(t, doc)
	if cfg.Users[0].Groups != nil {
		t.Errorf("expected nil groups for an empty string, got %v", cfg.Users[0].Groups)
	}
}

func TestParseRunCmdAliasNodeRejected(t *testing.T) {
	// A YAML alias reference is neither a scalar, a sequence, nor a map -
	// this is the only realistic way to reach kindName's default branch.
	doc := "runcmd:\n  - &a foo\n  - *a\n"
	_, err := Parse([]byte(doc))
	if err == nil {
		t.Fatal("expected an error for an alias-shaped runcmd entry, got nil")
	}
}

func TestParseErrorString(t *testing.T) {
	withLine := &ParseError{Line: 5, Msg: "boom"}
	if got := withLine.Error(); got != "line 5: boom" {
		t.Errorf("got %q", got)
	}

	withoutLine := &ParseError{Msg: "boom"}
	if got := withoutLine.Error(); got != "boom" {
		t.Errorf("got %q", got)
	}
}

func TestParseErrorFromLineFallbacks(t *testing.T) {
	cases := []string{
		"no line prefix here",
		"line not-a-number: boom",
		"line 5 no colon here",
	}
	for _, s := range cases {
		err := parseErrorFromLine(s)
		pe, ok := err.(*ParseError)
		if !ok {
			t.Fatalf("%q: got %T, want *ParseError", s, err)
		}
		if pe.Line != 0 || pe.Msg != s {
			t.Errorf("%q: got Line=%d Msg=%q, want Line=0 Msg=%q", s, pe.Line, pe.Msg, s)
		}
	}
}
