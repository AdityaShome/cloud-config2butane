# Problems and solutions

A record of the real problems hit while building this, mostly during the
Phase F demos  kept because the fixes aren't obvious in the code itself,
and some of them (the CAPD/kind environment issues especially) will bite
anyone else standing this up from scratch.

## Butane output was technically valid but unreadable (solved)

**Problem:** the first working version of `butaneout.Marshal` just called
`yaml.Marshal` on the assembled `butane1_1.Config`. Butane's own schema
types carry no `omitempty` tags, so every unset optional field came out
as `field: null` and every unset slice as `field: []`. A minimal user
entry printed 15 lines, 12 of them noise. File permissions also came out
as plain decimal (`mode: 420`) instead of the octal a human would
actually write (`mode: 0644`), since `0644` is just an `int` by the time
Go has parsed it and there's no record of how it was originally written.

**Fix:** encode into a `yaml.Node` tree instead of marshaling directly,
recursively prune any subtree that resolves to `!!null` or an empty
sequence/mapping, and reformat the `mode` key's value from decimal back
to a zero-padded octal string before the final marshal. See
`internal/butaneout/marshal.go`.

## A golden fixture's field order didn't match real output (solved)

**Problem:** early golden fixtures (`testdata/golden/*/expected.bu`) were
hand-written before `Convert`/`Marshal` existed, guessing at field order.
Once the real pipeline existed, `internal/convert/golden_test.go` was
added to run every fixture's `input.yaml` through `Parse -> Convert ->
Marshal` and diff the result  and it caught real order mismatches
immediately (Go struct field order, not alphabetical, is what
`yaml.Node.Encode` actually emits).

**Fix:** the golden test compares by unmarshaling both sides into
`interface{}` and using `reflect.DeepEqual`, not by comparing raw bytes.
Map key order isn't meaningful in YAML, so this is the correct comparison
regardless of how Butane's schema happens to declare its struct fields 
byte-exact comparison would be fragile against upstream reordering.

## `go install`ing the real `butane` CLI needed the right subpackage (solved)

**Problem:** `go install github.com/coreos/butane@v0.29.0` fails with
*"module found, but does not contain package"*  the module's root has
no `main` package. The actual CLI entrypoint lives at
`internal/main.go`, package `internal`.

**Fix:** `go install github.com/coreos/butane/internal@v0.29.0` builds
fine (Go's "internal package" import restriction only blocks *importing*
it from outside the module, not building it as a command)  it just
produces a binary literally named `internal`, which both demo scripts
rename to `butane` after install.

## `kubectl wait --for=condition=Ready nodes --all` gave a false PASS (solved)

**Problem:** the CAPI + kind demo's first working version waited for
node readiness with a single `kubectl wait ... --all` call, then
asserted PASS. It printed PASS  but the final `kubectl get nodes -o
wide` right below it showed the worker was still `NotReady`. `--all`
only snapshots whatever nodes already exist *when the command starts*;
the worker hadn't registered as a `Node` object yet at that instant
(control-plane comes up first, worker joins later), so the wait matched
only the control-plane node and returned immediately.

**Fix:** replaced the single `wait` call with a polling loop that checks
both the *count* of registered nodes and the *count* that are `Ready`,
looping until both reach the expected total or the timeout elapses. See
`e2e/capi-kind/run-demo.sh`. This is also why the demo script never trusts
a single-shot `wait` for anything whose object might not exist yet.

## CAPD couldn't reach the Docker daemon (solved)

**Problem:** after applying the workload cluster manifest, the `Cluster`
object sat in `Provisioning` forever. `kubectl describe cluster` showed:

```
Failed to create helper for managing the externalLoadBalancer: failed to
list containers: Cannot connect to the Docker daemon at
unix:///var/run/docker.sock. Is the docker daemon running?
```

Docker was running fine on the host  but CAPD's controller runs as a
*pod inside* the kind management cluster, and a bare `kind create
cluster` doesn't mount the host's Docker socket into the kind node. CAPD
has no path to the daemon it needs to create sibling containers for the
workload cluster.

**Fix:** create the kind management cluster with an explicit
`extraMounts` config mounting `/var/run/docker.sock` into the node (see
`e2e/capi-kind/kind-mgmt-config.yaml`). This is a standard, documented
CAPD + kind requirement, easy to miss if you don't already know it's
Docker-socket dependent.

Cleaning up after this mistake taught a second lesson: deleting the kind
cluster *before* deleting the CAPI `Cluster` object orphans CAPD's
workload containers, since the controller that would have torn them down
is gone. The demo script now always deletes the `Cluster` object first
and waits for it, then deletes the kind cluster.

## `envsubst` doesn't support `${VAR:-default}` (solved)

**Problem:** the upstream CAPD cluster template
(`manifests/bases/*.yaml`) uses bash-style default-value substitution:
`preLoadImages: ${DOCKER_PRELOAD_IMAGES:-[]}`. Piping the rendered
kustomize output through GNU `envsubst` left that token completely
untouched  `envsubst` only understands plain `${VAR}`, not the `:-`
fallback syntax  so the literal string `${DOCKER_PRELOAD_IMAGES:-[]}`
got sent to the API server, which rejected it: `preLoadImages: Invalid
value: "string": ... must be of type array`.

**Fix:** since this is the one place the templates rely on
`envsubst`-unsupported syntax, it's handled with a targeted `sed` after
`envsubst` runs: `sed 's/\${DOCKER_PRELOAD_IMAGES:-\[\]}/[]/'`.

## Host inotify limit exhausted under nested kind + CAPD (solved)

**Problem:** even after fixing the Docker socket mount, the workload
cluster's worker `Node` registered but never went `Ready`:

```
Ready  False  KubeletNotReady  container runtime network not ready:
NetworkReady=false ... cni plugin not initialized
```

`kube-proxy` on that node was `CrashLoopBackOff`, logging:

```
"command failed" err="failed complete: too many open files"
```

Not a real file-descriptor limit (`ulimit -n` was 1,048,576)  it was
`fs.inotify.max_user_instances`, which defaults to 128 on Linux and is
shared across every process the host user owns. Running kind (a
Kubernetes cluster in Docker) *and* CAPD (which spins up more containers,
each running their own kubelet/containerd/kube-proxy with their own
watchers) from inside that one kind cluster exhausts 128 fast.

**Fix:** raise the host limit 

```sh
sudo sysctl fs.inotify.max_user_instances=8192 fs.inotify.max_user_watches=524288
```

 documented as a hard prerequisite in `e2e/capi-kind/README.md`, since
there's no way to work around it from inside the containers.

## `clusterctl generate cluster` doesn't work for the Docker provider (solved)

**Problem:** `clusterctl generate cluster ... --infrastructure docker`
fails: *"failed to download files from GitHub release: failed to get
file cluster-template.yaml"*. Unlike most infrastructure providers, CAPD
doesn't publish ready-to-use templates as release assets  it's meant
for local development/testing, and its templates only exist as kustomize
bases in the CAPI source tree's own e2e test data.

**Fix:** fetch `test/e2e/data/infrastructure-docker/main/{bases,cluster-
template-ignition}/*.yaml` directly from `kubernetes-sigs/cluster-api` at
the git tag matching the installed provider version, vendor them into
`e2e/capi-kind/manifests/`, and drive them with `kubectl kustomize | env-
subst` instead of `clusterctl generate`.

## CAPI's `additionalConfig` ignition field isn't Butane (solved)

**Problem (a design question, not a bug):** the plan called for feeding
this tool's generated Butane/Ignition into CAPI as the worker's bootstrap
data. Reading CAPI's actual source
(`bootstrap/kubeadm/pkg/ignition/clc/clc.go`) showed that
`KubeadmConfig`'s `format: ignition` path is parsed exclusively by the
legacy `flatcar/container-linux-config-transpiler`, targeting **Ignition
spec v2.3**  a different, older schema than the **Ignition v3.4** this
tool produces via Butane. There's no raw-Ignition or Butane passthrough
anywhere in that code path; a real integration would need something
outside CAPI's built-in bootstrap provider entirely.

**Resolution:** rather than silently building something that looked like
it satisfied the plan but didn't actually exercise this tool's output,
this was raised directly and the demo's scope was deliberately narrowed:
the CAPI + kind demo proves the *infrastructure* side (a real,
ignition-formatted worker can be built and joined by Cluster API, using
CAPI's own native CLC path), while the QEMU demo is what proves *this
tool's own Butane output* is correct  against a real Flatcar Ignition
consumer, end to end. See the scope note in `e2e/capi-kind/README.md`.

## `sudo: false` produced a nonsensical sudoers line (solved)

**Problem:** cloud-init's `sudo` field accepts the literal boolean
`false` to mean "no sudo for this user." Because a YAML boolean is still
a scalar node, the original `StringOrList` decoder for `sudo` treated it
like any other scalar and kept its raw text  so `sudo: false` parsed as
the one-element list `["false"]`, which the users converter then wrote
out as a real sudoers.d line: `alice false`. Verified empirically before
touching anything, to make sure the bug was real and not a
misunderstanding of the decoder:

```sh
$ cloud-config2butane -in - -validate=false <<< '#cloud-config
users:
  - name: alice
    sudo: false'
...
storage:
  files:
    - path: /etc/sudoers.d/90-cloud-config2butane
      contents:
        inline: |
          alice false
```

Initially left documented as a known gap rather than fixed under time
pressure, then fixed once there was room to do it properly.

**Fix:** replaced the generic `StringOrList` type (which `sudo` was the
only user of) with a dedicated `SudoRules` type in
`internal/cloudconfig/types.go` whose `UnmarshalYAML` checks the YAML
node's tag. A `!!bool` tag with value `false` now decodes to no rules at
all (no sudoers file gets generated for that user); a `!!bool` tag with
value `true`  which cloud-init's own docs don't assign any meaning to 
is a hard parse error rather than a guess:

```sh
$ cloud-config2butane -in - -validate=false <<< '#cloud-config
users:
  - name: alice
    sudo: true'
parsing -:
line 4: sudo: "true" is not a supported value (use a rule string, a list of rules, or false)
```

Confirmed both cases empirically before and after the fix, not just via
the new unit tests (`TestParseUserSudoFalseMeansNoSudo`,
`TestParseUserSudoTrueRejected`).
