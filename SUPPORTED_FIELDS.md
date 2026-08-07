# Supported fields

Every cloud-config field this transpiler understands, what it becomes in Butane,
and how any shape ambiguity in the original feature matrix was resolved.
Anything not listed here is rejected at parse time with an explicit
`unsupported field: <key>` error  see [OUTSCOPE.md](OUTSCOPE.md).

## Users

| cloud-config | Butane | Notes |
|---|---|---|
| `users[].name` | `passwd.users[].name` | `name: default` is resolved to `core`, Flatcar's built-in default user, rather than creating a new user. |
| `users[].groups` | `passwd.users[].groups` | Accepts a comma-separated string (`"wheel, docker"`) or a YAML list; both normalize to the same list. |
| `users[].sudo` | *(no direct field  see below)* | Accepts a single rule string, a list of rule strings, or `false` (no sudo access  cloud-init's own documented meaning for that value). `true` has no defined meaning for this field and is a hard error rather than a guess. |
| `users[].shell` | `passwd.users[].shell` | Direct pass-through. |
| `users[].ssh_authorized_keys` | `passwd.users[].ssh_authorized_keys` | Direct pass-through; see the SSH keys section below for how this combines with the top-level field. |
| `users[].passwd` | `passwd.users[].password_hash` | Must already be a crypt(3) hash (starts with `$`)  Ignition has no way to hash a plaintext password at provisioning time, so a plaintext value is a hard error, not a warning. |
| `users[].lock_passwd` | *(folds into `password_hash`)* | Defaults to `true`. When locked, the hash is prefixed with `!`, the same convention `passwd -l` uses. `lock_passwd: false` leaves the hash unprefixed. |
| `users[].gecos` | `passwd.users[].gecos` | Direct pass-through. |
| `users[].primary_group` | `passwd.users[].primary_group` | Direct pass-through. |
| `users[].no_create_home` | `passwd.users[].no_create_home` | Direct pass-through. |

**`sudo` has no Butane/Ignition equivalent.** It's rendered as a single generated
drop-in file, `/etc/sudoers.d/90-cloud-config2butane` (mode `0440`), with one line
per user per rule: `<username> <rule>`. Every user with a `sudo` entry contributes
to this same file, matching how cloud-init itself writes a single combined
sudoers.d drop-in.

## Groups

| cloud-config | Butane | Notes |
|---|---|---|
| `groups` (top-level) | `passwd.groups[]` | Accepts a list of bare names (`docker`) and/or a list of single-key maps (`admingroup: [root, sys]`) in any combination. |

**Group membership from the map form is best-effort.** Butane's `passwd.groups[]`
has no "initial members" field  the only way to put a user in a group is via
that user's own `passwd.users[].groups`. So a member listed under a top-level
group is added to that user's `groups` *only if* the same username also appears
in the cloud-config's own `users:` list. A member with no matching `users:` entry
has no user record to attach the group to; the group itself is still created, but
that particular membership is silently not applied. See OUTSCOPE.md.

## SSH keys

| cloud-config | Butane | Notes |
|---|---|---|
| `ssh_authorized_keys` (top-level) | merged into `passwd.users[].ssh_authorized_keys` | Cloud-init documents this as applying "to the default user." We merge it into whichever user resolves to `core` (see the `name: default` resolution above), listed before that user's own per-user keys. It is **not** merged into any other user. |
| `users[].ssh_authorized_keys` | `passwd.users[].ssh_authorized_keys` | Direct pass-through, additive with the top-level keys for the default user. |

## Files

| cloud-config | Butane | Notes |
|---|---|---|
| `write_files[].path` | `storage.files[].path` | Direct pass-through. |
| `write_files[].content` | `storage.files[].contents.inline` (or `.append`, see below) | Decoded per `encoding` before embedding. |
| `write_files[].owner` | `storage.files[].user`/`.group` | `"user:group"` is split on the first `:`; either half may be omitted (`"root"` sets only the user, `":wheel"` sets only the group). |
| `write_files[].permissions` | `storage.files[].mode` | Accepts a quoted octal string (`"0644"`) or a bare number. The raw scalar text is parsed as base-8 directly, so `644` and `0644` both mean the same thing regardless of YAML's own int-vs-string resolution. |
| `write_files[].encoding` | *(controls decoding, not a field itself)* | `""`/`text/plain`, `base64`/`b64`, `gzip`/`gz`, and `gzip+base64`/`gz+base64`/`gzip+b64`/`gz+b64` are all decoded back to plain bytes and embedded as literal inline content  nothing stays base64- or gzip-encoded in the output. Any other value is a hard error. |
| `write_files[].append` | `storage.files[].append` | Maps directly onto Butane's own native `append` field, which Ignition supports at provisioning time  no read-modify-write emulation needed. |

**Certificates and private keys are not a separate cloud-config field**  they're
just `write_files` entries. Any file whose decoded content contains the text
`PRIVATE KEY` (i.e. looks like PEM key material) has its mode forced to `0600`,
overriding whatever `permissions` was declared in the source. This is a
deliberate safety decision: a cloud-config author who wrote looser permissions
on key material almost certainly didn't mean to.

## systemd units (pass-through)

| cloud-config | Butane | Notes |
|---|---|---|
| `write_files[]` targeting a systemd unit path | `systemd.units[]` | Detected, not a separate field. |

**Detection rule:** a `write_files` entry is treated as a systemd unit, not a
generic file, when its path is directly inside `/etc/systemd/system/` (not a
subdirectory  drop-in override directories like `foo.service.d/override.conf`
stay generic files) and its filename ends in `.service`, `.socket`, `.timer`,
`.path`, `.mount`, or `.target`. Matched units are written with `enabled: true`
by default, so their own `[Install]` section actually takes effect  a unit
file dropped in place without being enabled would otherwise sit inert.

## runcmd / bootcmd

| cloud-config | Butane | Notes |
|---|---|---|
| `runcmd` | generated script (`/opt/cloud-config2butane/runcmd.sh`) + `systemd.units[]` oneshot (`cloud-config2butane-runcmd.service`) | Runs **once**, on first boot: `WantedBy=multi-user.target` (which only starts after Ignition has finished writing every `storage.files` entry), guarded by `ConditionPathExists=!/var/lib/cloud-config2butane/runcmd.done` and a matching `ExecStartPost` touch, so it never re-runs on later boots. |
| `bootcmd` | generated script (`/opt/cloud-config2butane/bootcmd.sh`) + `systemd.units[]` oneshot (`cloud-config2butane-bootcmd.service`) | Runs on **every** boot, early: `DefaultDependencies=no`, `Before=sysinit.target`, `WantedBy=sysinit.target`, no guard file. Its `sysinit.target` unit necessarily starts before runcmd's `multi-user.target` unit in normal boot order, so no explicit ordering between the two was added. |

Both accept a list of plain shell-line strings and/or argv-form lists
(`[mkdir, -p, /opt/data]`), which may be mixed in the same list. Argv entries
are shell-quoted when flattened into the script. The generated script runs with
`set -euo pipefail`  it stops at the first failing command, which is a
deliberate departure from cloud-init's own behavior of logging a failed command
and continuing to the next one.

## hostname / fqdn

| cloud-config | Butane | Notes |
|---|---|---|
| `hostname` | `storage.files[]` writing `/etc/hostname` | Wins over `fqdn` if both are set. |
| `fqdn` | `storage.files[]` writing `/etc/hostname` | Used only when `hostname` is absent; the short hostname is everything before the first `.`. |
