# E2E and Acceptance Tests

This repo uses two different "end-to-end" layers on purpose:

- `make test-e2e` (Go/Ginkgo): lightweight smoke e2e
  - Goal: validate the controller can be built, deployed into a fresh kind cluster, and serves metrics.
  - Fast and stable enough to run in CI on every PR.

- Acceptance tests (shell): deeper end-to-end checks
  - `make test-acceptance-kro-reflection`: kro demo flow (kro -> RGD -> Kany8sControlPlane reflection)
    - legacy alias: `make test-acceptance`
  - `make test-acceptance-kro-infra-reflection`: kro infra demo flow (kro -> RGD -> Kany8sCluster reflection)
  - `make test-acceptance-kro-reflection-multi-rgd`: kro demo flow with 2 RGDs (proves `Kany8sControlPlane` can drive multiple instance kinds)
    - legacy alias: `make test-acceptance-multi-rgd`
  - `make test-acceptance-capd-kubeadm`: CAPD + kubeadm workload provisioning (real API server)
    - legacy alias: `make test-acceptance-self-managed`

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

### Acceptance tests (shell)

This repo intentionally keeps acceptance checks as shell scripts so the command sequence stays close to the human-readable guides.

#### 1) kro demo flow (managed control plane reflection)

- Command: `make test-acceptance-kro-reflection` (runs `bash hack/acceptance-test-kro-reflection.sh`)
- Scope: automate the `e2e-guide.md` demo on a fresh kind cluster:
  1) Create a fresh kind cluster
  2) Install kro v0.7.1
  3) Apply kro RBAC workaround (unrestricted aggregation ClusterRole)
  4) Apply demo RGD `test/acceptance_test/manifests/kro/rgd.yaml` and wait for:
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
  - Note: demo RGD is not a real API server, so `RemoteConnectionProbe=False` / `Cluster Available=False` is expected.

#### 2) self-managed flow (CAPD + kubeadm)

- Command: `make test-acceptance-capd-kubeadm` (runs `bash hack/acceptance-test-capd-kubeadm.sh`)
- Scope:
  - Install Cluster API providers (CAPD + CABPK) via clusterctl
  - Deploy Kany8s (including `Kany8sKubeadmControlPlane`)
  - Render + apply `test/acceptance_test/manifests/self-managed-docker/cluster.yaml.tpl` (equivalent to `examples/self-managed-docker/cluster.yaml`)
  - Wait for `RemoteConnectionProbe=True` and `Available=True`
  - Fetch workload kubeconfig and verify connectivity

## Acceptance Script Design (`hack/acceptance-test-kro-reflection.sh`)

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

## Makefile integration

Preferred targets:

- `make test-acceptance-kro-reflection` -> `bash hack/acceptance-test-kro-reflection.sh`
- `make test-acceptance-kro-reflection-keep` -> `CLEANUP=false bash hack/acceptance-test-kro-reflection.sh`
- `make test-acceptance-kro-infra-reflection` -> `bash hack/acceptance-test-kro-infra-reflection.sh`
- `make test-acceptance-kro-infra-reflection-keep` -> `CLEANUP=false bash hack/acceptance-test-kro-infra-reflection.sh`
- `make test-acceptance-kro-reflection-multi-rgd` -> `bash hack/acceptance-test-kro-reflection-multi-rgd.sh`
- `make test-acceptance-kro-reflection-multi-rgd-keep` -> `CLEANUP=false bash hack/acceptance-test-kro-reflection-multi-rgd.sh`
- `make test-acceptance-capd-kubeadm` -> `bash hack/acceptance-test-capd-kubeadm.sh`
- `make test-acceptance-capd-kubeadm-keep` -> `CLEANUP=false bash hack/acceptance-test-capd-kubeadm.sh`

Legacy aliases are still supported:

- `make test-acceptance` -> `make test-acceptance-kro-reflection`
- `make test-acceptance-multi-rgd` -> `make test-acceptance-kro-reflection-multi-rgd`
- `make test-acceptance-self-managed` -> `make test-acceptance-capd-kubeadm`

## References

- Manual guide (source of truth for acceptance flow): `e2e-guide.md`
- Troubleshooting checklist: `docs/runbooks/e2e.md`
- Demo RGD (acceptance): `test/acceptance_test/manifests/kro/rgd.yaml`
- Demo RGD (manual): `examples/kro/ready-endpoint/rgd.yaml`
- Optional CAPI sample: `examples/capi/cluster.yaml`
- Existing smoke e2e: `test/e2e/e2e_test.go`
