package convert

import (
	"strings"

	butane "github.com/coreos/butane/base/v0_5"

	"github.com/AdityaShome/cloud-config2butane/internal/cloudconfig"
)

const (
	runCmdScriptPath = "/opt/cloud-config2butane/runcmd.sh"
	runCmdUnitName   = "cloud-config2butane-runcmd.service"

	bootCmdScriptPath = "/opt/cloud-config2butane/bootcmd.sh"
	bootCmdUnitName   = "cloud-config2butane-bootcmd.service"
)

// runCmdUnitContents runs the generated script once, on first boot only.
// WantedBy=multi-user.target already guarantees it starts after Ignition
// has finished writing storage.files; the marker file stops it running
// again on later boots, matching cloud-init's runcmd semantics.
const runCmdUnitContents = `[Unit]
Description=Run cloud-config runcmd once
ConditionPathExists=!/var/lib/cloud-config2butane/runcmd.done

[Service]
Type=oneshot
ExecStartPre=/usr/bin/mkdir -p /var/lib/cloud-config2butane
ExecStart=` + runCmdScriptPath + `
ExecStartPost=/usr/bin/touch /var/lib/cloud-config2butane/runcmd.done
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
`

// bootCmdUnitContents runs the generated script on every boot, early,
// before sysinit.target - which also puts it ahead of runCmd's
// multi-user.target unit in normal boot order, so no explicit ordering
// between the two units is needed.
const bootCmdUnitContents = `[Unit]
Description=Run cloud-config bootcmd every boot
DefaultDependencies=no
After=systemd-remount-fs.service
Before=sysinit.target

[Service]
Type=oneshot
ExecStart=` + bootCmdScriptPath + `
RemainAfterExit=yes

[Install]
WantedBy=sysinit.target
`

// RunCmd converts `runcmd` into a generated shell script plus the oneshot
// unit that runs it once. Returns nil, nil if there's nothing to do.
func RunCmd(commands cloudconfig.CommandList) (*butane.File, *butane.Unit) {
	return commandsToUnitAndFile(commands, runCmdScriptPath, runCmdUnitName, runCmdUnitContents)
}

// BootCmd converts `bootcmd` the same way, but bootCmdUnitContents gives
// it every-boot, early-boot semantics instead of runcmd's once-only,
// multi-user.target semantics.
func BootCmd(commands cloudconfig.CommandList) (*butane.File, *butane.Unit) {
	return commandsToUnitAndFile(commands, bootCmdScriptPath, bootCmdUnitName, bootCmdUnitContents)
}

func commandsToUnitAndFile(commands cloudconfig.CommandList, scriptPath, unitName, unitContents string) (*butane.File, *butane.Unit) {
	if len(commands) == 0 {
		return nil, nil
	}

	script := buildScript(commands)
	file := &butane.File{
		Path:     scriptPath,
		Mode:     intPtr(0755),
		Contents: butane.Resource{Inline: &script},
	}
	unit := &butane.Unit{
		Name:     unitName,
		Enabled:  boolPtr(true),
		Contents: strPtr(unitContents),
	}
	return file, unit
}

// buildScript flattens commands into a shell script. It fails fast
// (set -euo pipefail) rather than cloud-init's own behavior of logging a
// failed command and moving on - a deliberate, documented departure in
// favor of a provisioning script that doesn't silently continue in a
// half-configured state.
func buildScript(commands []cloudconfig.Command) string {
	var b strings.Builder
	b.WriteString("#!/bin/bash\nset -euo pipefail\n\n")
	for _, c := range commands {
		if c.IsArgv() {
			parts := make([]string, len(c.Argv))
			for i, a := range c.Argv {
				parts[i] = shellQuote(a)
			}
			b.WriteString(strings.Join(parts, " "))
		} else {
			b.WriteString(c.Line)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	for _, r := range s {
		if !isSafeUnquoted(r) {
			return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
		}
	}
	return s
}

func isSafeUnquoted(r rune) bool {
	if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
		return true
	}
	return strings.ContainsRune("-_./:=@%,+", r)
}
