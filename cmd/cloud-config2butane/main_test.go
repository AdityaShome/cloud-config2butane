package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunWritesValidatedButaneToFile(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "cloud-config.yaml")
	out := filepath.Join(dir, "config.bu")
	if err := os.WriteFile(in, []byte("#cloud-config\nhostname: node1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := run(in, out, true); err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "variant: flatcar") {
		t.Errorf("got %s, want a flatcar butane doc", data)
	}
}

func TestRunFailsOnMissingInput(t *testing.T) {
	if err := run(filepath.Join(t.TempDir(), "missing.yaml"), "", true); err == nil {
		t.Fatal("expected an error for a missing input file")
	}
}

func TestRunFailsOnUnsupportedField(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "cloud-config.yaml")
	if err := os.WriteFile(in, []byte("#cloud-config\npackages:\n  - htop\n"), 0644); err != nil {
		t.Fatal(err)
	}
	err := run(in, "", true)
	if err == nil || !strings.Contains(err.Error(), "unsupported field: packages") {
		t.Fatalf("got %v, want an unsupported field error", err)
	}
}

func TestRunFailsOnPlaintextPasswd(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "cloud-config.yaml")
	if err := os.WriteFile(in, []byte("#cloud-config\nusers:\n  - name: alice\n    passwd: hunter2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	err := run(in, "", true)
	if err == nil || !strings.Contains(err.Error(), "plaintext") {
		t.Fatalf("got %v, want a plaintext passwd error", err)
	}
}

func TestRunValidateFalseSkipsButaneCheck(t *testing.T) {
	// A relative storage path is something our own converter doesn't
	// check for - Ignition itself requires absolute paths - so this
	// only fails when real butane validation actually runs, proving
	// -validate=false skips it.
	dir := t.TempDir()
	in := filepath.Join(dir, "cloud-config.yaml")
	if err := os.WriteFile(in, []byte("#cloud-config\nwrite_files:\n  - path: etc/motd\n    content: x\n"), 0644); err != nil {
		t.Fatal(err)
	}

	errValidated := run(in, "", true)
	if errValidated == nil {
		t.Fatal("expected validation to catch the relative path")
	}

	if err := run(in, "", false); err != nil {
		t.Fatalf("expected -validate=false to skip the check that fails otherwise, got %v", err)
	}
}

func TestRunWritesToStdoutWhenOutIsEmpty(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "cloud-config.yaml")
	if err := os.WriteFile(in, []byte("#cloud-config\nhostname: node1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := run(in, "", true); err != nil {
		t.Fatalf("run: %v", err)
	}
}
