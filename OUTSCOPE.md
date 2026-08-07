## Out of scope by design

Flatcar is image-based and has no package manager, which drives most of this
list. These are not gaps to fill in later  they're deliberate non-goals that
match the scope of the upstream issue this transpiler was built for.

- Package management (`packages`, `apt`, `yum`, etc.)  Flatcar is image-based,
  no package manager.
- Disk partitioning / filesystem creation DSL (`disk_setup`, `fs_setup`,
  `mounts`) beyond what's needed to place files.
- Power state / reboot directives (`power_state`).
- Network configuration (`network`, `write_network_config`).
- Any cloud-config module not needed for ClusterAPI worker bootstrap.

Every unsupported top-level cloud-config key produces a named, explicit error
(`unsupported field: <key>`) at parse time. Nothing is silently dropped.

## Gaps found during implementation

These weren't part of the original scoping but came up while building the
converters. Each is a deliberate choice, documented rather than silently
guessed at  see [SUPPORTED_FIELDS.md](SUPPORTED_FIELDS.md) for the full
resolution of each field.

- **Group membership only reaches users also defined in `users:`.** Butane has
  no "initial members" concept on `passwd.groups[]`  membership can only be
  expressed on the member's own `passwd.users[].groups`. A member named under a
  top-level group with no matching `users:` entry has no user record to attach
  the group to, so that specific membership is dropped (the group itself is
  still created).
- **Private key material always gets `mode: 0600`,** overriding whatever
  `permissions` the source declared, whenever the decoded file content contains
  the literal text `PRIVATE KEY`. This is a safety-first override, not a bug 
  but it means a file that happens to contain that string in a comment or
  fixture, without actually being a private key, will also get its mode forced.
- **`runcmd`/`bootcmd` scripts fail fast** (`set -euo pipefail`): the first
  failing command stops the rest of the script. cloud-init's own runcmd module
  logs a failed command and moves on to the next one. This transpiler picked
  the stricter behavior deliberately, on the assumption that continuing
  provisioning in a known-broken state is worse than stopping.
- **systemd unit detection is path/suffix-based only.** A `write_files` entry
  under `/etc/systemd/system/` ending in a recognized unit suffix is passed
  through to `systemd.units[]` and enabled by default  its contents are never
  parsed or validated as an actual unit file. A syntactically broken unit will
  still be accepted by this transpiler and only fail later, at boot.
- **Only four content encodings are supported** for `write_files`: none
  (`text/plain`), `base64`/`b64`, `gzip`/`gz`, and the `gzip+base64` family.
  Anything else is a hard error.
