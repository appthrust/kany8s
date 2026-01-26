# E2E and Acceptance Tests

This repo uses two different "end-to-end" layers on purpose:

- `make test-e2e` (Go/Ginkgo): lightweight smoke e2e
  - Goal: validate the controller can be built, deployed into a fresh kind cluster, and serves metrics.
  - Fast and stable enough to run in CI on every PR.

- `hack/acceptance-test.sh` + `make test-acceptance`: acceptance-level e2e
  - Goal: automate the exact demo flow documented in `e2e-guide.md` (kro -> RGD acceptance -> Kany8s -> Kany8sControlPlane reflection).
  - Shell-based so the "source of truth" stays the guide's concrete kubectl/kind/make commands.

This split keeps "CI e2e" small while still having a reproducible acceptance script for deeper verification.

## What Each Layer Covers

### Unit / envtest

- Command: `make test`
- Scope: controller logic + helpers + contracts (fast feedback).

### Smoke e2e (existing)

- Command: `make test-e2e`
- Scope:
  - build manager image
  - deploy controller-manager in kind
  - verify controller pod runs
  - verify metrics endpoint returns `200 OK`
- Non-goals:
  - installing kro
  - applying demo RGDs
  - verifying `Kany8sControlPlane` reflects kro status

### Acceptance test

- Command: `make test-acceptance` (will run `bash hack/acceptance-test.sh`)
- Scope: automate `e2e-guide.md` end-to-end demo on a fresh kind cluster:
  1) Create a fresh kind cluster
  2) Install kro v0.7.1
  3) Apply kro RBAC workaround (unrestricted aggregation ClusterRole)
  4) Apply demo RGD `examples/kro/ready-endpoint/rgd.yaml` and wait for:
     - `ResourceGraphAccepted=True`
     - generated instance CRD exists: `democontrolplanes.kro.run`
  5) Install + deploy Kany8s in-cluster (CRDs + controller image load + deploy)
  6) Apply `Kany8sControlPlane` and verify the full kro -> Kany8s reflection:
     - kro instance is created 1:1 (same name/namespace)
     - kro instance normalized status: `status.ready=true` and `status.endpoint != ""`
     - Kany8s reflects:
       - `Kany8sControlPlane.spec.controlPlaneEndpoint.{host,port}`
       - `Kany8sControlPlane.status.initialization.controlPlaneInitialized=true`
       - `Kany8sControlPlane Ready=True`

- Optional scope (off by default): Cluster API endpoint mirroring
  - Install/upgrade CAPI providers to v1beta2 via `clusterctl v1.12.2`
  - Apply `examples/capi/cluster.yaml`
  - Verify `Cluster.spec.controlPlaneEndpoint` is mirrored from `Kany8sControlPlane`
  - Note: demo RGD is not a real API server, so `RemoteConnectionProbe=False` / `Cluster Available=False` is expected (see `e2e-guide.md`).

## Acceptance Script Design (`hack/acceptance-test.sh`)

### Inputs (environment variables)

- `KIND_CLUSTER_NAME`:
  - default: `kany8s-acceptance-<timestamp>`
- `KUBECTL_CONTEXT`:
  - default: `kind-${KIND_CLUSTER_NAME}`
- `KRO_VERSION`:
  - default: `0.7.1`
- `IMG` (controller image tag):
  - default: `example.com/kany8s:acceptance`
- `NAMESPACE` / `CLUSTER_NAME`:
  - defaults: `default` / `demo-cluster`
- `CLEANUP`:
  - default: `true` (delete kind cluster at the end)
- `ARTIFACTS_DIR`:
  - default: `/tmp/kany8s-acceptance-<timestamp>`
- `WITH_CAPI`:
  - default: `false` (only when true run clusterctl + apply `examples/capi/cluster.yaml`)
- `CLUSTERCTL_VERSION`:
  - default: `v1.12.2` (only used when `WITH_CAPI=true`)

### Behavior requirements

- Always use explicit context (`kubectl --context ...`) to avoid leaking into the user's current kubeconfig state.
- Be strict bash: `set -euo pipefail`.
- On failure, collect debugging artifacts before exiting:
  - `kubectl get/describe` for key resources
  - `kubectl get events -A --sort-by=.metadata.creationTimestamp`
  - logs:
    - `kubectl -n kany8s-system logs deploy/kany8s-controller-manager -c manager`
    - `kubectl -n kro-system logs deploy/kro`
- Keep the repo clean:
  - `make deploy` mutates `config/manager/kustomization.yaml`; if inside a git repo, restore it at the end:
    - `git restore config/manager/kustomization.yaml`
  - This should be best-effort and must not hide unrelated local changes.

### Assertions (acceptance-level)

- kro:
  - `rgd/demo-control-plane.kro.run` reaches `ResourceGraphAccepted=True`
  - generated CRD exists: `democontrolplanes.kro.run`
- Kany8s:
  - controller-manager Deployment rolls out
  - `Kany8sControlPlane/<CLUSTER_NAME>` reaches `Ready=True`
  - kro instance `<kind=DemoControlPlane>/<CLUSTER_NAME>` exists and:
    - `.status.ready == true`
    - `.status.endpoint` is non-empty
  - `Kany8sControlPlane` reflects:
    - `.spec.controlPlaneEndpoint.host` non-empty
    - `.spec.controlPlaneEndpoint.port` non-zero
    - `.status.initialization.controlPlaneInitialized == true`

### Optional assertions (when `WITH_CAPI=true`)

- Cluster CRD serves v1beta2 contract (`clusters.cluster.x-k8s.io` includes `v1beta2:true,true`)
- Apply infraRef object required by the sample:
  - `infrastructure.cluster.x-k8s.io/v1alpha1` `Kany8sCluster/<CLUSTER_NAME>`
- Apply `examples/capi/cluster.yaml`
- Wait for `Cluster.spec.controlPlaneEndpoint.host` to equal the expected host.
- Do NOT fail the script just because `Cluster Available=False` (expected for demo).

## Makefile Integration

Add targets:

- `make test-acceptance` -> `bash hack/acceptance-test.sh`
- (optional convenience) `make test-acceptance-keep` -> `CLEANUP=false make test-acceptance`

## References

- Manual guide (source of truth for acceptance flow): `e2e-guide.md`
- Troubleshooting checklist: `docs/runbooks/e2e.md`
- Demo RGD: `examples/kro/ready-endpoint/rgd.yaml`
- Optional CAPI sample: `examples/capi/cluster.yaml`
- Existing smoke e2e: `test/e2e/e2e_test.go`
