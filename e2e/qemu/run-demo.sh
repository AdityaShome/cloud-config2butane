#!/usr/bin/env bash
# End-to-end QEMU demo: cloud-config -> cloud-config2butane -> butane ->
# ignition -> boot a real Flatcar VM -> SSH in and assert the generated
# config actually took effect.
#
# Prerequisites: qemu-system-x86_64, KVM access, ssh/ssh-keygen, curl,
# bzip2, go, and network access (to fetch the Flatcar image on first run
# and the butane CLI if it isn't already on PATH).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
CACHE_DIR="${CACHE_DIR:-$SCRIPT_DIR/.cache}"
WORK_DIR="$(mktemp -d)"
SSH_PORT="${SSH_PORT:-2222}"
FLATCAR_BASE_URL="https://stable.release.flatcar-linux.net/amd64-usr/current"
QEMU_PID=""

log() { echo "[qemu-demo] $*" >&2; }
fail() { log "FAIL: $*"; exit 1; }

cleanup() {
  if [ -n "$QEMU_PID" ] && kill -0 "$QEMU_PID" 2>/dev/null; then
    log "stopping VM (pid $QEMU_PID)"
    kill "$QEMU_PID" 2>/dev/null || true
    wait "$QEMU_PID" 2>/dev/null || true
  fi
  rm -rf "$WORK_DIR"
}
trap cleanup EXIT

mkdir -p "$CACHE_DIR"

if [ ! -f "$CACHE_DIR/flatcar_production_qemu_image.img" ]; then
  log "downloading Flatcar QEMU image (cached in $CACHE_DIR for next run)"
  curl -sL -o "$CACHE_DIR/flatcar_production_qemu.sh" \
    "$FLATCAR_BASE_URL/flatcar_production_qemu.sh"
  curl -sL -o "$CACHE_DIR/flatcar_production_qemu_image.img.bz2" \
    "$FLATCAR_BASE_URL/flatcar_production_qemu_image.img.bz2"
  bzip2 -dk "$CACHE_DIR/flatcar_production_qemu_image.img.bz2"
fi
chmod +x "$CACHE_DIR/flatcar_production_qemu.sh"

log "building cloud-config2butane"
(cd "$REPO_ROOT" && go build -o "$WORK_DIR/cloud-config2butane" ./cmd/cloud-config2butane)

BUTANE_BIN="$(command -v butane || true)"
if [ -z "$BUTANE_BIN" ]; then
  log "butane CLI not found on PATH, building it via go install"
  GOBIN="$WORK_DIR" go install github.com/coreos/butane/internal@v0.29.0
  mv "$WORK_DIR/internal" "$WORK_DIR/butane"
  BUTANE_BIN="$WORK_DIR/butane"
fi

log "generating an ephemeral SSH keypair for this run"
ssh-keygen -t ed25519 -N "" -f "$WORK_DIR/id_ed25519" -q -C cloud-config2butane-demo
SSH_PUBKEY="$(cat "$WORK_DIR/id_ed25519.pub")"
sed "s#__SSH_PUBLIC_KEY__#$SSH_PUBKEY#" "$SCRIPT_DIR/cloud-config.yaml.tmpl" > "$WORK_DIR/cloud-config.yaml"

log "converting cloud-config -> butane (validated against real butane)"
"$WORK_DIR/cloud-config2butane" -in "$WORK_DIR/cloud-config.yaml" -out "$WORK_DIR/config.bu"

log "compiling butane -> ignition with the real butane CLI"
"$BUTANE_BIN" --pretty --strict -o "$WORK_DIR/config.ign" "$WORK_DIR/config.bu"

log "booting Flatcar VM"
"$CACHE_DIR/flatcar_production_qemu.sh" \
  -I "$CACHE_DIR/flatcar_production_qemu_image.img" \
  -i "$WORK_DIR/config.ign" \
  -p "$SSH_PORT" \
  -nographic \
  > "$WORK_DIR/qemu.log" 2>&1 &
QEMU_PID=$!

SSH_OPTS=(-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o ConnectTimeout=3 -o BatchMode=yes -i "$WORK_DIR/id_ed25519" -p "$SSH_PORT")

log "waiting for SSH (up to 5 minutes)"
ready=0
for _ in $(seq 1 60); do
  if ssh "${SSH_OPTS[@]}" core@127.0.0.1 true 2>/dev/null; then
    ready=1
    break
  fi
  sleep 5
done
if [ "$ready" -ne 1 ]; then
  log "VM never became reachable over SSH; qemu log follows"
  cat "$WORK_DIR/qemu.log" >&2
  fail "SSH never came up"
fi

run() { ssh "${SSH_OPTS[@]}" core@127.0.0.1 "$@"; }

log "asserting hostname"
[ "$(run hostname)" = "demo-worker" ] || fail "hostname mismatch"

log "asserting the core user exists"
run id core >/dev/null || fail "core user missing"

log "asserting write_files content and mode"
run sudo test -f /etc/kubernetes/kubeadm-join-config.yaml || fail "kubeadm config file missing"
run sudo grep -q JoinConfiguration /etc/kubernetes/kubeadm-join-config.yaml || fail "kubeadm config content wrong"
mode="$(run sudo stat -c '%a' /etc/kubernetes/kubeadm-join-config.yaml)"
[ "$mode" = "600" ] || fail "expected mode 600 on the kubeadm config, got $mode"

log "asserting runcmd ran (once, on first boot)"
run sudo test -f /var/lib/cloud-config2butane-demo/runcmd-ran || fail "runcmd marker missing"

log "asserting bootcmd ran"
run sudo test -f /var/lib/cloud-config2butane-demo-bootcmd-ran || fail "bootcmd marker missing"

log "PASS: all assertions succeeded"
