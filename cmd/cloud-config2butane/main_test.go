package main

import (
	"errors"
	"fmt"
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

func TestFlattenErrorsSingle(t *testing.T) {
	err := errors.New("boom")
	got := flattenErrors(err)
	if len(got) != 1 || got[0] != "boom" {
		t.Errorf("got %v", got)
	}
}

func TestFlattenErrorsWrappedSingle(t *testing.T) {
	err := fmt.Errorf("reading file: %w", errors.New("no such file"))
	got := flattenErrors(err)
	if len(got) != 1 || got[0] != "no such file" {
		t.Errorf("got %v, want the innermost error unwrapped", got)
	}
}

func TestFlattenErrorsJoinedMultiple(t *testing.T) {
	err := errors.Join(errors.New("first"), errors.New("second"))
	got := flattenErrors(err)
	want := []string{"first", "second"}
	if !equalStrings(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFlattenErrorsWrappedJoin(t *testing.T) {
	// matches run()'s own shape: fmt.Errorf("converting %s:\n%w", in,
	// errors.Join(errs...))
	joined := errors.Join(errors.New("user: missing name"), errors.New("bad passwd"))
	err := fmt.Errorf("converting x.yaml:\n%w", joined)
	got := flattenErrors(err)
	want := []string{"user: missing name", "bad passwd"}
	if !equalStrings(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFlattenErrorsNil(t *testing.T) {
	if got := flattenErrors(nil); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
