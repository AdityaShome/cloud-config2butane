package convert

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/AdityaShome/cloud-config2butane/internal/cloudconfig"
)

func scalarNode(t *testing.T, yamlValue string) *yaml.Node {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(yamlValue), &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Content[0]
}

func TestFilesPlain(t *testing.T) {
	files := []cloudconfig.File{
		{Path: "/etc/motd", Content: "hello\n"},
	}
	out, _, errs := Files(files)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if out[0].Path != "/etc/motd" {
		t.Errorf("got path %q", out[0].Path)
	}
	if out[0].Contents.Inline == nil || *out[0].Contents.Inline != "hello\n" {
		t.Errorf("got contents %v", out[0].Contents.Inline)
	}
}

func TestFilesBase64(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("hello\n"))
	files := []cloudconfig.File{
		{Path: "/etc/motd", Content: encoded, Encoding: "b64"},
	}
	out, _, errs := Files(files)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if *out[0].Contents.Inline != "hello\n" {
		t.Errorf("got %q, want decoded content", *out[0].Contents.Inline)
	}
}

func gzipBytes(t *testing.T, s string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write([]byte(s)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestFilesGzipBase64(t *testing.T) {
	compressed := gzipBytes(t, "hello\n")
	encoded := base64.StdEncoding.EncodeToString(compressed)
	files := []cloudconfig.File{
		{Path: "/etc/motd", Content: encoded, Encoding: "gz+b64"},
	}
	out, _, errs := Files(files)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if *out[0].Contents.Inline != "hello\n" {
		t.Errorf("got %q, want decoded content", *out[0].Contents.Inline)
	}
}

func TestFilesGzipOnly(t *testing.T) {
	compressed := gzipBytes(t, "hello\n")
	files := []cloudconfig.File{
		{Path: "/etc/motd", Content: string(compressed), Encoding: "gzip"},
	}
	out, _, errs := Files(files)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if *out[0].Contents.Inline != "hello\n" {
		t.Errorf("got %q, want decoded content", *out[0].Contents.Inline)
	}
}

func TestFilesUnsupportedEncoding(t *testing.T) {
	files := []cloudconfig.File{
		{Path: "/etc/motd", Content: "x", Encoding: "rot13"},
	}
	_, _, errs := Files(files)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "unsupported encoding") {
		t.Fatalf("got %v", errs)
	}
}

func TestFilesOwnerUserAndGroup(t *testing.T) {
	files := []cloudconfig.File{
		{Path: "/etc/motd", Owner: "root:wheel"},
	}
	out, _, _ := Files(files)
	if out[0].User.Name == nil || *out[0].User.Name != "root" {
		t.Errorf("got user %v", out[0].User.Name)
	}
	if out[0].Group.Name == nil || *out[0].Group.Name != "wheel" {
		t.Errorf("got group %v", out[0].Group.Name)
	}
}

func TestFilesOwnerUserOnly(t *testing.T) {
	files := []cloudconfig.File{
		{Path: "/etc/motd", Owner: "root"},
	}
	out, _, _ := Files(files)
	if out[0].User.Name == nil || *out[0].User.Name != "root" {
		t.Errorf("got user %v", out[0].User.Name)
	}
	if out[0].Group.Name != nil {
		t.Errorf("expected no group, got %v", *out[0].Group.Name)
	}
}

func TestFilesPermissionsPassthrough(t *testing.T) {
	var mode cloudconfig.FileMode
	if err := (&mode).UnmarshalYAML(scalarNode(t, `"0644"`)); err != nil {
		t.Fatal(err)
	}
	files := []cloudconfig.File{
		{Path: "/etc/motd", Permissions: mode},
	}
	out, _, _ := Files(files)
	if out[0].Mode == nil || *out[0].Mode != 0644 {
		t.Errorf("got mode %v, want 0644", out[0].Mode)
	}
}

func TestFilesPermissionsUnsetLeavesModeNil(t *testing.T) {
	files := []cloudconfig.File{{Path: "/etc/motd"}}
	out, _, _ := Files(files)
	if out[0].Mode != nil {
		t.Errorf("expected nil mode, got %v", *out[0].Mode)
	}
}

func TestFilesAppend(t *testing.T) {
	files := []cloudconfig.File{
		{Path: "/etc/hosts", Content: "127.0.0.1 extra\n", Append: true},
	}
	out, _, _ := Files(files)
	if out[0].Contents.Inline != nil {
		t.Errorf("expected Contents to be empty when appending, got %v", out[0].Contents.Inline)
	}
	if len(out[0].Append) != 1 || out[0].Append[0].Inline == nil || *out[0].Append[0].Inline != "127.0.0.1 extra\n" {
		t.Errorf("got append %v", out[0].Append)
	}
}

func TestFilesPrivateKeyForcesMode0600(t *testing.T) {
	key := "-----BEGIN RSA PRIVATE KEY-----\nMIIBogIBAAJ...\n-----END RSA PRIVATE KEY-----\n"
	var loose cloudconfig.FileMode
	if err := (&loose).UnmarshalYAML(scalarNode(t, `"0644"`)); err != nil {
		t.Fatal(err)
	}
	files := []cloudconfig.File{
		{Path: "/etc/ssl/key.pem", Content: key, Permissions: loose},
	}
	out, _, _ := Files(files)
	if out[0].Mode == nil || *out[0].Mode != 0600 {
		t.Errorf("got mode %v, want forced 0600", out[0].Mode)
	}
}

func TestFilesSystemdUnitPassthrough(t *testing.T) {
	unitContents := "[Unit]\nDescription=demo\n\n[Service]\nExecStart=/usr/bin/true\n\n[Install]\nWantedBy=multi-user.target\n"
	files := []cloudconfig.File{
		{Path: "/etc/systemd/system/demo.service", Content: unitContents},
	}
	outFiles, outUnits, errs := Files(files)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(outFiles) != 0 {
		t.Errorf("expected no generic files, got %v", outFiles)
	}
	if len(outUnits) != 1 {
		t.Fatalf("expected 1 unit, got %v", outUnits)
	}
	u := outUnits[0]
	if u.Name != "demo.service" {
		t.Errorf("got unit name %q", u.Name)
	}
	if u.Enabled == nil || !*u.Enabled {
		t.Errorf("expected unit to be enabled by default")
	}
	if u.Contents == nil || *u.Contents != unitContents {
		t.Errorf("got contents %v", u.Contents)
	}
}

func TestFilesSystemdDropinNotTreatedAsUnit(t *testing.T) {
	files := []cloudconfig.File{
		{Path: "/etc/systemd/system/demo.service.d/override.conf", Content: "[Service]\n"},
	}
	outFiles, outUnits, errs := Files(files)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(outUnits) != 0 {
		t.Errorf("expected drop-in to stay a generic file, got units %v", outUnits)
	}
	if len(outFiles) != 1 || outFiles[0].Path != "/etc/systemd/system/demo.service.d/override.conf" {
		t.Errorf("got files %v", outFiles)
	}
}

func TestFilesSystemdUnitOutsideUnitDirStaysGenericFile(t *testing.T) {
	files := []cloudconfig.File{
		{Path: "/opt/demo.service", Content: "not a real unit"},
	}
	outFiles, outUnits, _ := Files(files)
	if len(outUnits) != 0 {
		t.Errorf("expected no units for a path outside %s, got %v", systemdUnitDir, outUnits)
	}
	if len(outFiles) != 1 {
		t.Errorf("expected the file to pass through as a generic file, got %v", outFiles)
	}
}

func TestFilesMissingPath(t *testing.T) {
	files := []cloudconfig.File{{Content: "x"}}
	out, _, errs := Files(files)
	if len(out) != 0 {
		t.Errorf("expected pathless file to be skipped, got %v", out)
	}
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %v", errs)
	}
}
