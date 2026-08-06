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
	want := StringOrList{"ALL=(ALL) NOPASSWD:ALL"}

	for _, doc := range []string{stringForm, listForm} {
		cfg := mustParse(t, doc)
		if !reflect.DeepEqual(cfg.Users[0].Sudo, want) {
			t.Errorf("doc %q: got sudo %v, want %v", doc, cfg.Users[0].Sudo, want)
		}
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
