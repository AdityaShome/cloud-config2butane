package validate

import (
	"strings"
	"testing"

	"github.com/AdityaShome/cloud-config2butane/internal/butaneout"
	"github.com/AdityaShome/cloud-config2butane/internal/cloudconfig"
	"github.com/AdityaShome/cloud-config2butane/internal/convert"
)

func TestIgnitionAcceptsValidConfig(t *testing.T) {
	doc := []byte(`
variant: flatcar
version: 1.1.0
passwd:
  users:
    - name: alice
      shell: /bin/bash
`)
	if err := Ignition(doc); err != nil {
		t.Fatalf("expected a valid config to pass, got %v", err)
	}
}

func TestIgnitionRejectsBadVariant(t *testing.T) {
	doc := []byte(`
variant: not-a-real-variant
version: 1.1.0
`)
	if err := Ignition(doc); err == nil {
		t.Fatal("expected an error for an unknown variant, got nil")
	}
}

func TestIgnitionRejectsBadFieldValue(t *testing.T) {
	doc := []byte(`
variant: flatcar
version: 1.1.0
storage:
  files:
    - path: /etc/motd
      mode: not-a-number
`)
	if err := Ignition(doc); err == nil {
		t.Fatal("expected an error for a non-numeric mode, got nil")
	}
}

func TestIgnitionAcceptsOurOwnConverterOutput(t *testing.T) {
	cfg, err := cloudconfig.Parse([]byte(`
#cloud-config
hostname: worker-01
users:
  - name: alice
    groups: wheel, docker
    shell: /bin/bash
    ssh_authorized_keys:
      - ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI alice@example.com
write_files:
  - path: /etc/motd
    permissions: "0644"
    content: hello
runcmd:
  - systemctl restart sshd
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	butaneCfg, errs := convert.Convert(cfg)
	if len(errs) != 0 {
		t.Fatalf("Convert: %v", errs)
	}
	out, err := butaneout.Marshal(butaneCfg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := Ignition(out); err != nil {
		t.Fatalf("real butane rejected our own generated config: %v\n\ngenerated:\n%s", err, out)
	}
}

func TestIgnitionErrorMentionsButane(t *testing.T) {
	doc := []byte(`variant: bogus`)
	err := Ignition(doc)
	if err == nil || !strings.Contains(err.Error(), "butane") {
		t.Fatalf("got %v, want an error mentioning butane", err)
	}
}
