#!/usr/bin/env bash
# End-to-end CAPI + kind demo: stands up a kind management cluster,
# installs Cluster API with the Docker infrastructure provider (CAPD),
# and provisions a workload cluster whose control plane and worker are
# both bootstrapped through CAPI's native kubeadm+ignition (CLC) path -
# proving a Flatcar-shaped, ignition-formatted node can be built and
# joined by real Cluster API machinery, not just asserted about.
#
# Scope note: this demo does not feed cloud-config2butane's own Butane
# output through CAPI. CAPI's built-in ignition bootstrap format is
# parsed by the legacy flatcar/container-linux-config-transpiler (Ignition
# spec v2.3), not Butane (Ignition spec v3.x) - there's no passthrough for
# our tool's actual output anywhere in that path. That our tool's output
# is itself correct, real bootstrap data is what ../qemu/ proves, against
# a real Flatcar Ignition consumer. See README.md in this directory.
#
# Prerequisites: docker, kind, kubectl, clusterctl, curl, envsubst,
# network access (to pull kindest/node images on first run).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-capi-mgmt}"
WORKLOAD_CLUSTER_NAME="${WORKLOAD_CLUSTER_NAME:-capi-demo}"
KUBERNETES_VERSION="${KUBERNETES_VERSION:-v1.27.3}"
WORK_DIR="$(mktemp -d)"
KEEP_CLUSTER="${KEEP_CLUSTER:-0}"

log() { echo "[capi-kind-demo] $*" >&2; }
fail() { log "FAIL: $*"; exit 1; }

cleanup() {
  if [ "$KEEP_CLUSTER" = "1" ]; then
    log "KEEP_CLUSTER=1 set, leaving the cluster up for inspection"
    log "workload kubeconfig: $WORK_DIR/workload-kubeconfig"
    return
  fi
  log "cleaning up"
  # Delete the Cluster first so CAPD's own controller tears down its
  # workload containers; deleting the kind cluster out from under CAPD
  # would orphan them instead.
  kubectl delete cluster "$WORKLOAD_CLUSTER_NAME" --ignore-not-found=true --wait=true --timeout=120s 2>/dev/null || true
  kind delete cluster --name "$KIND_CLUSTER_NAME" 2>/dev/null || true
  rm -rf "$WORK_DIR"
}
trap cleanup EXIT

log "creating kind management cluster (with the docker socket mounted in, which CAPD requires)"
kind create cluster --name "$KIND_CLUSTER_NAME" --config "$SCRIPT_DIR/kind-mgmt-config.yaml"

export CLUSTER_TOPOLOGY=false
export EXP_KUBEADM_BOOTSTRAP_FORMAT_IGNITION=true
log "installing Cluster API (docker infrastructure provider, ignition feature gate on)"
clusterctl init --infrastructure docker
kubectl wait --for=condition=Available deployment --all -A --timeout=180s

log "rendering the workload cluster manifest"
(
  cd "$SCRIPT_DIR/manifests"
  export CLUSTER_NAME="$WORKLOAD_CLUSTER_NAME"
  export KUBERNETES_VERSION
  export CONTROL_PLANE_MACHINE_COUNT=1
  export WORKER_MACHINE_COUNT=1
  export DOCKER_SERVICE_CIDRS="10.128.0.0/12"
  export DOCKER_POD_CIDRS="192.168.0.0/16"
  export DOCKER_SERVICE_DOMAIN="cluster.local"
  # envsubst doesn't support bash's ${VAR:-default} syntax, and this is
  # the only place the upstream template relies on it.
  kubectl kustomize . | envsubst | sed 's/\${DOCKER_PRELOAD_IMAGES:-\[\]}/[]/' > "$WORK_DIR/cluster.yaml"
)

log "applying the workload cluster"
kubectl apply -f "$WORK_DIR/cluster.yaml"

log "waiting for the control plane to initialize (up to 5 minutes)"
kubectl wait --for=condition=ControlPlaneInitialized "cluster/$WORKLOAD_CLUSTER_NAME" --timeout=300s

log "fetching the workload cluster's kubeconfig"
clusterctl get kubeconfig "$WORKLOAD_CLUSTER_NAME" > "$WORK_DIR/workload-kubeconfig"

log "installing kindnet so nodes can go Ready"
KUBECONFIG="$WORK_DIR/workload-kubeconfig" kubectl apply -f "$SCRIPT_DIR/manifests/cni/kindnet.yaml"

log "waiting for both nodes to register and become Ready (up to 5 minutes)"
# Not `kubectl wait --for=condition=Ready nodes --all`: --all only
# snapshots nodes that already exist when the command starts, and the
# worker can still be mid-join at that point, so a wait issued too early
# would silently succeed on the control plane alone.
expected_nodes=2
ready=0
for _ in $(seq 1 60); do
  total="$(KUBECONFIG="$WORK_DIR/workload-kubeconfig" kubectl get nodes --no-headers 2>/dev/null | wc -l)"
  ready_count="$(KUBECONFIG="$WORK_DIR/workload-kubeconfig" kubectl get nodes --no-headers 2>/dev/null | awk '$2 == "Ready"' | wc -l)"
  if [ "$total" -ge "$expected_nodes" ] && [ "$ready_count" -ge "$expected_nodes" ]; then
    ready=1
    break
  fi
  sleep 5
done
KUBECONFIG="$WORK_DIR/workload-kubeconfig" kubectl get nodes -o wide
[ "$ready" -eq 1 ] || fail "expected $expected_nodes Ready nodes, didn't get there in time"

log "asserting the worker actually went through the ignition/CLC bootstrap path"
worker_container="$(KUBECONFIG="$WORK_DIR/workload-kubeconfig" kubectl get nodes -l '!node-role.kubernetes.io/control-plane' -o jsonpath='{.items[0].metadata.name}')"
marker="$(docker exec "$worker_container" cat /opt/capi-ignition-marker 2>/dev/null || true)"
[ -n "$marker" ] || fail "ignition marker file missing on the worker - ignition/CLC path did not run as expected"
log "marker found: $marker"

log "PASS: control plane + worker both Ready, worker bootstrapped via CAPI's native ignition path"
KUBECONFIG="$WORK_DIR/workload-kubeconfig" kubectl get nodes -o wide
