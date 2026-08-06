package butaneout

import (
	"strings"
	"testing"

	butane "github.com/coreos/butane/base/v0_5"
	butane1_1 "github.com/coreos/butane/config/flatcar/v1_1"
)

func TestMarshalOmitsUnsetFields(t *testing.T) {
	cfg := &butane1_1.Config{
		Config: butane.Config{
			Version: "1.1.0",
			Variant: "flatcar",
			Passwd: butane.Passwd{
				Users: []butane.PasswdUser{{Name: "alice"}},
			},
		},
	}
	out, err := Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, unwanted := range []string{"null", "gecos", "shell", "storage", "systemd", "groups: []"} {
		if strings.Contains(s, unwanted) {
			t.Errorf("output should omit unset fields, but contains %q:\n%s", unwanted, s)
		}
	}
}

func TestMarshalUsesTwoSpaceIndent(t *testing.T) {
	cfg := &butane1_1.Config{
		Config: butane.Config{
			Version: "1.1.0",
			Variant: "flatcar",
			Passwd: butane.Passwd{
				Users: []butane.PasswdUser{{Name: "alice"}},
			},
		},
	}
	out, err := Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "  users:") {
		t.Errorf("expected 2-space indent, got:\n%s", out)
	}
	if strings.Contains(string(out), "    users:") {
		t.Errorf("expected 2-space indent, not 4, got:\n%s", out)
	}
}

func TestMarshalFormatsModeAsOctal(t *testing.T) {
	mode := 0644
	content := "x"
	cfg := &butane1_1.Config{
		Config: butane.Config{
			Version: "1.1.0",
			Variant: "flatcar",
			Storage: butane.Storage{
				Files: []butane.File{
					{Path: "/etc/motd", Mode: &mode, Contents: butane.Resource{Inline: &content}},
				},
			},
		},
	}
	out, err := Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "mode: 0644") {
		t.Errorf("expected octal mode formatting, got:\n%s", out)
	}
}

func TestMarshalEmptyConfigOmitsAllSections(t *testing.T) {
	cfg := &butane1_1.Config{
		Config: butane.Config{Version: "1.1.0", Variant: "flatcar"},
	}
	out, err := Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := "version: 1.1.0\nvariant: flatcar\n"
	if string(out) != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}
