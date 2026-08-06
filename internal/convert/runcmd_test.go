package convert

import (
	"strings"
	"testing"

	"github.com/AdityaShome/cloud-config2butane/internal/cloudconfig"
)

func TestRunCmdEmpty(t *testing.T) {
	file, unit := RunCmd(nil)
	if file != nil || unit != nil {
		t.Fatalf("expected nil, nil for empty runcmd, got %v, %v", file, unit)
	}
}

func TestRunCmdSimpleLine(t *testing.T) {
	cmds := cloudconfig.CommandList{{Line: "systemctl restart sshd"}}
	file, unit := RunCmd(cmds)
	if file == nil || unit == nil {
		t.Fatal("expected a file and a unit")
	}
	if !strings.Contains(*file.Contents.Inline, "systemctl restart sshd\n") {
		t.Errorf("script missing command: %q", *file.Contents.Inline)
	}
	if file.Mode == nil || *file.Mode != 0755 {
		t.Errorf("got mode %v, want 0755", file.Mode)
	}
	if unit.Name != runCmdUnitName {
		t.Errorf("got unit name %q", unit.Name)
	}
	if unit.Enabled == nil || !*unit.Enabled {
		t.Errorf("expected unit to be enabled")
	}
}

func TestRunCmdArgvFormIsShellQuoted(t *testing.T) {
	cmds := cloudconfig.CommandList{
		{Argv: []string{"mkdir", "-p", "/opt/data with spaces"}},
	}
	file, _ := RunCmd(cmds)
	script := *file.Contents.Inline
	want := "mkdir -p '/opt/data with spaces'"
	if !strings.Contains(script, want) {
		t.Errorf("script %q missing quoted argv line %q", script, want)
	}
}

func TestRunCmdArgvWithSingleQuotesEscaped(t *testing.T) {
	cmds := cloudconfig.CommandList{
		{Argv: []string{"echo", "it's here"}},
	}
	file, _ := RunCmd(cmds)
	script := *file.Contents.Inline
	want := `echo 'it'\''s here'`
	if !strings.Contains(script, want) {
		t.Errorf("script %q missing escaped argv line %q", script, want)
	}
}

func TestRunCmdOnceSemantics(t *testing.T) {
	cmds := cloudconfig.CommandList{{Line: "echo hi"}}
	_, unit := RunCmd(cmds)
	if !strings.Contains(*unit.Contents, "WantedBy=multi-user.target") {
		t.Errorf("expected runcmd unit to target multi-user.target: %s", *unit.Contents)
	}
	if !strings.Contains(*unit.Contents, "ConditionPathExists=!") {
		t.Errorf("expected a once-only guard condition: %s", *unit.Contents)
	}
}

func TestBootCmdEveryBootSemantics(t *testing.T) {
	cmds := cloudconfig.CommandList{{Line: "echo hi"}}
	_, unit := BootCmd(cmds)
	if unit.Name != bootCmdUnitName {
		t.Errorf("got unit name %q", unit.Name)
	}
	if !strings.Contains(*unit.Contents, "WantedBy=sysinit.target") {
		t.Errorf("expected bootcmd unit to target sysinit.target: %s", *unit.Contents)
	}
	if strings.Contains(*unit.Contents, "ConditionPathExists=!") {
		t.Errorf("bootcmd must not have a once-only guard, it should run every boot: %s", *unit.Contents)
	}
}

func TestRunCmdAndBootCmdAreIndependent(t *testing.T) {
	runFile, runUnit := RunCmd(cloudconfig.CommandList{{Line: "echo run"}})
	bootFile, bootUnit := BootCmd(cloudconfig.CommandList{{Line: "echo boot"}})

	if runFile.Path == bootFile.Path {
		t.Errorf("expected distinct script paths, both are %q", runFile.Path)
	}
	if runUnit.Name == bootUnit.Name {
		t.Errorf("expected distinct unit names, both are %q", runUnit.Name)
	}
}

func TestShellQuoteLeavesSafeStringsUnquoted(t *testing.T) {
	for _, s := range []string{"mkdir", "-p", "/opt/data", "kube-node-01", "a=b"} {
		if got := shellQuote(s); got != s {
			t.Errorf("shellQuote(%q) = %q, want unquoted", s, got)
		}
	}
}

func TestShellQuoteHandlesEmptyString(t *testing.T) {
	if got := shellQuote(""); got != "''" {
		t.Errorf("shellQuote(\"\") = %q, want ''", got)
	}
}
