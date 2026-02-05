# Codebase Documentation: Kany8s

This document is a code-oriented map of the repository. It focuses on:

- What the project is and how it is structured
- Where the core behavior lives (CRDs, controllers, internal packages)
- Major functions with file/line references
- Basic size/LOC metrics

Notes:

- File/line references (e.g. `internal/controller/kany8scontrolplane_controller.go:98`) are based on the current repository state and will shift as code changes.
- Auto-generated files are called out explicitly; do not edit them by hand.

## Quick Facts

- Go module: `github.com/reoring/kany8s` (`go.mod:1`, 303 lines)
- Go version/toolchain: `go 1.25.0` / `toolchain go1.25.5` (`go.mod:3`, `go.mod:5`)
- Core dependencies:
  - Cluster API v1beta2 (`sigs.k8s.io/cluster-api v1.12.2`, `go.mod:17`)
  - controller-runtime (`sigs.k8s.io/controller-runtime v0.23.0`, `go.mod:18`)
  - Kubernetes API/client-go v0.35.0 (`go.mod:13-16`)
  - kro (consumes `kro.run` CRDs via dynamic/unstructured access)

## What Kany8s Is

Kany8s is a work-in-progress Cluster API provider suite that aims to provision control planes (and eventually infrastructure) in a provider-agnostic way by delegating provider-specific realization to kro RGDs (ResourceGraphDefinition).

High-level idea (from `docs/README.md`):

- Cluster API-facing CRDs: `Kany8sCluster` (Infrastructure) + `Kany8sControlPlane` (ControlPlane)
- Provider-specific realization: a kro `ResourceGraphDefinition` that produces an instance CR; Kany8s creates exactly one instance per control plane and watches only the instance status

## Repository Layout

Top-level:

- `cmd/` - manager entrypoint (`cmd/main.go`)
- `api/` - API types (CRD schemas) and generated DeepCopy code
- `internal/` - controllers, helpers, devtools
- `config/` - kustomize manifests, generated CRDs/RBAC, samples
- `dist/` - generated install bundle (`dist/install.yaml`)
- `examples/` - example YAMLs (kro, CAPI, self-managed)
- `hack/` - acceptance scripts and tooling
- `test/` - e2e tests and helpers
- `docs/` - docs (PRD, ADR, guides, runbooks, reference, archive)

Go packages (from `go list ./...`):

- `github.com/reoring/kany8s/cmd`
- `github.com/reoring/kany8s/api/v1alpha1`
- `github.com/reoring/kany8s/api/infrastructure/v1alpha1`
- `github.com/reoring/kany8s/internal/controller`
- `github.com/reoring/kany8s/internal/controller/controlplane`
- `github.com/reoring/kany8s/internal/controller/infrastructure`
- `github.com/reoring/kany8s/internal/dynamicwatch`
- `github.com/reoring/kany8s/internal/endpoint`
- `github.com/reoring/kany8s/internal/kro`
- `github.com/reoring/kany8s/internal/kubeconfig`
- `github.com/reoring/kany8s/internal/constants`
- `github.com/reoring/kany8s/internal/devtools` (repo-policy tests)
- `github.com/reoring/kany8s/test/utils` (e2e helpers)

## Build / Run / Test Entry Points

Make targets live in `Makefile` (266 lines):

- Unit/envtest: `make test` (`Makefile:60-63`)
- E2E (kind-based): `make test-e2e` (`Makefile:84-88`)
- Acceptance:
  - kro reflection (managed control plane): `make test-acceptance-kro-reflection` -> `hack/acceptance-test-kro-reflection.sh`
    - legacy alias: `make test-acceptance`
  - kro infra reflection (infrastructure): `make test-acceptance-kro-infra-reflection` -> `hack/acceptance-test-kro-infra-reflection.sh`
    - keep cluster: `make test-acceptance-kro-infra-reflection-keep`
    - wrapper runner: `test/acceptance_test/run-acceptance-kro-infra-reflection.sh`
  - kro infra cluster identity (clusterUID / ownerReferences): `make test-acceptance-kro-infra-cluster-identity` -> `hack/acceptance-test-kro-infra-cluster-identity.sh`
    - keep cluster: `make test-acceptance-kro-infra-cluster-identity-keep`
    - wrapper runner: `test/acceptance_test/run-acceptance-kro-infra-cluster-identity.sh`
  - kro reflection (multi RGD / multi instance kind): `make test-acceptance-kro-reflection-multi-rgd` -> `hack/acceptance-test-kro-reflection-multi-rgd.sh`
    - legacy alias: `make test-acceptance-multi-rgd`
  - CAPD + kubeadm (self-managed): `make test-acceptance-capd-kubeadm` -> `hack/acceptance-test-capd-kubeadm.sh`
    - legacy alias: `make test-acceptance-self-managed`
- Local run: `make run` (`Makefile:127-129`) -> `go run ./cmd/main.go`
- Build/install/deploy:
  - `make build` (`Makefile:123-126`) -> `bin/manager`
  - `make install` / `make deploy` (`Makefile:171-185`)
  - `make build-installer` (`Makefile:159-164`) -> `dist/install.yaml`

## APIs (CRDs)

This repo defines two API groups (both `v1alpha1`):

- ControlPlane group: `controlplane.cluster.x-k8s.io/v1alpha1` (`api/v1alpha1/groupversion_info.go`, 37 lines)
- Infrastructure group: `infrastructure.cluster.x-k8s.io/v1alpha1` (`api/infrastructure/v1alpha1/groupversion_info.go`, 37 lines)

Key kinds and their Go types:

- ControlPlane:
  - `Kany8sControlPlane` (`api/v1alpha1/kany8scontrolplane_types.go`, 144 lines)
  - `Kany8sControlPlaneTemplate` (`api/v1alpha1/kany8scontrolplanetemplate_types.go`, 91 lines)
  - `Kany8sKubeadmControlPlane` (`api/v1alpha1/kany8skubeadmcontrolplane_types.go`, 146 lines)
- Infrastructure:
  - `Kany8sCluster` (`api/infrastructure/v1alpha1/kany8scluster_types.go`, 131 lines)
  - `Kany8sClusterTemplate` (`api/infrastructure/v1alpha1/kany8sclustertemplate_types.go`, 89 lines)
  - `ResourceGraphDefinitionReference` (`api/infrastructure/v1alpha1/resourcegraphdefinitionreference_types.go`, 27 lines)

Generated (do not edit):

- `api/v1alpha1/zz_generated.deepcopy.go` (generated, 435 lines)
- `api/infrastructure/v1alpha1/zz_generated.deepcopy.go` (generated, 291 lines)

### Field Highlights (contracts)

Managed control plane (kro mode):

- `Kany8sControlPlane.spec.version` (required) (`api/v1alpha1/kany8scontrolplane_types.go:39-41`)
- `Kany8sControlPlane.spec.resourceGraphDefinitionRef.name` (required) (`api/v1alpha1/kany8scontrolplane_types.go:43-46`)
- `Kany8sControlPlane.spec.kroSpec` (arbitrary JSON passthrough) (`api/v1alpha1/kany8scontrolplane_types.go:47-51`)
- `Kany8sControlPlane.spec.controlPlaneEndpoint` (set by controller) (`api/v1alpha1/kany8scontrolplane_types.go:52-56`)
- `Kany8sControlPlane.status.initialization.controlPlaneInitialized` (`api/v1alpha1/kany8scontrolplane_types.go:58-65`)
- `Kany8sControlPlane.status.version` is part of the CAPI v1beta2 control plane contract (`api/v1alpha1/kany8scontrolplane_types.go:69-77`)

Self-managed control plane (kubeadm mode):

- `Kany8sKubeadmControlPlane.spec.machineTemplate.infrastructureRef` (`api/v1alpha1/kany8skubeadmcontrolplane_types.go:28-33`)
- `Kany8sKubeadmControlPlane.spec.kubeadmConfigSpec` (`api/v1alpha1/kany8skubeadmcontrolplane_types.go:52-55`)
- `Kany8sKubeadmControlPlane.spec.controlPlaneEndpoint` (set from infra endpoint) (`api/v1alpha1/kany8skubeadmcontrolplane_types.go:56-60`)

Infrastructure:

- `Kany8sCluster.status.initialization.provisioned` is part of the CAPI v1beta2 InfrastructureCluster contract (`api/infrastructure/v1alpha1/kany8scluster_types.go:41-49`)
- Optional kro-driven infra via `Kany8sCluster.spec.resourceGraphDefinitionRef` (`api/infrastructure/v1alpha1/kany8scluster_types.go:27-39`)

## Manager Wiring (entrypoint)

The controller-manager binary is configured in `cmd/main.go` (235 lines).

Major functions:

- `init()` registers schemes (`cmd/main.go:55`)
- `main()` wires logging, TLS options, metrics/webhook servers, and controllers, then starts the manager (`cmd/main.go:66`)

Controllers registered by the manager (`cmd/main.go:196-217`):

- `controller.Kany8sControlPlaneReconciler` (`internal/controller/kany8scontrolplane_controller.go`)
- `infrastructurecontroller.Kany8sClusterReconciler` (`internal/controller/infrastructure/kany8scluster_controller.go`)
- `controlplanecontroller.Kany8sKubeadmControlPlaneReconciler` (`internal/controller/controlplane/kany8skubeadmcontrolplane_controller.go`)

Notable manager settings:

- Client cache disables caching for Secrets (`cmd/main.go:74-78`)
- Healthz/Readyz endpoints enabled (`cmd/main.go:220-227`)

## Controllers (Reconciliation Logic)

### 1) Managed ControlPlane via kro: `Kany8sControlPlaneReconciler`

File: `internal/controller/kany8scontrolplane_controller.go` (518 lines)

Major functions and line numbers:

- `(*Kany8sControlPlaneReconciler).Reconcile(...)` (`internal/controller/kany8scontrolplane_controller.go:98`)
- `(*Kany8sControlPlaneReconciler).reconcileKubeconfigSecret(...)` (`internal/controller/kany8scontrolplane_controller.go:218`)
- `(*Kany8sControlPlaneReconciler).reconcileConditionsAndFailure(...)` (`internal/controller/kany8scontrolplane_controller.go:344`)
- `buildKroInstanceSpec(...)` (`internal/controller/kany8scontrolplane_controller.go:423`)
- `(*Kany8sControlPlaneReconciler).SetupWithManager(...)` (`internal/controller/kany8scontrolplane_controller.go:498`)

Core behavior (Reconcile flow):

1. Load `Kany8sControlPlane`.
2. Resolve the target kro instance GVK from the referenced RGD (`kro.ResolveInstanceGVK`, `internal/kro/gvk.go:15`).
3. Ensure a dynamic watch exists for that instance GVK (via `dynamicwatch.Watcher`, see `internal/dynamicwatch/watcher.go`).
4. CreateOrUpdate an unstructured kro instance with 1:1 naming (`cp.Name`/`cp.Namespace`) and owner reference.
5. Build instance spec by parsing `spec.kroSpec` JSON and injecting `spec.version` (`buildKroInstanceSpec`, `internal/controller/kany8scontrolplane_controller.go:423`).
6. Read the kro instance normalized status (`kro.ReadInstanceStatus`, `internal/kro/status.go:16`).
7. If `status.kubeconfigSecretRef` exists, copy/validate kubeconfig into the CAPI-compatible `<cluster>-kubeconfig` Secret (`reconcileKubeconfigSecret`, `internal/controller/kany8scontrolplane_controller.go:218`).
8. Parse/validate the endpoint string and update `Kany8sControlPlane.spec.controlPlaneEndpoint` (`endpoint.Parse`, `internal/endpoint/parse.go:13`).
9. Set `status.initialization.controlPlaneInitialized=true` once endpoint is known.
10. Reconcile conditions/failure fields and requeue while not ready.

Topology/version contract is covered by `internal/controller/cluster_topology_contract_test.go` (188 lines):

- `TestClusterTopologyVersionChangePropagatesToKroInstance` (`internal/controller/cluster_topology_contract_test.go:26`) verifies that when `Kany8sControlPlane.spec.version` changes, the kro instance `spec.version` is updated accordingly.

### 2) Infrastructure provider: `Kany8sClusterReconciler`

File: `internal/controller/infrastructure/kany8scluster_controller.go` (205 lines)

Major functions:

- `(*Kany8sClusterReconciler).Reconcile(...)` (`internal/controller/infrastructure/kany8scluster_controller.go:56`)
- `buildKroInstanceSpec(...)` (`internal/controller/infrastructure/kany8scluster_controller.go:172`)
- `(*Kany8sClusterReconciler).SetupWithManager(...)` (`internal/controller/infrastructure/kany8scluster_controller.go:199`)

Core behavior:

- Stub mode (default): sets `status.initialization.provisioned=true` and `Ready=True` to unblock CAPI flows.
- Optional kro mode: if `spec.resourceGraphDefinitionRef` is set, it resolves the RGD instance GVK, creates/updates a 1:1 instance, and uses the instance `status.ready`/`status.reason`/`status.message` to drive `Provisioned` and `Ready`.

### 3) Self-managed ControlPlane (CAPD + kubeadm): `Kany8sKubeadmControlPlaneReconciler`

File: `internal/controller/controlplane/kany8skubeadmcontrolplane_controller.go` (695 lines)

Major functions:

- `(*Kany8sKubeadmControlPlaneReconciler).Reconcile(...)` (`internal/controller/controlplane/kany8skubeadmcontrolplane_controller.go:84`)
- `(*Kany8sKubeadmControlPlaneReconciler).reconcileReadiness(...)` (`internal/controller/controlplane/kany8skubeadmcontrolplane_controller.go:195`)
- `(*Kany8sKubeadmControlPlaneReconciler).reconcileInitialControlPlaneKubeadmConfig(...)` (`internal/controller/controlplane/kany8skubeadmcontrolplane_controller.go:332`)
- `(*Kany8sKubeadmControlPlaneReconciler).reconcileInfraMachine(...)` (`internal/controller/controlplane/kany8skubeadmcontrolplane_controller.go:416`)
- `(*Kany8sKubeadmControlPlaneReconciler).reconcileControlPlaneMachine(...)` (`internal/controller/controlplane/kany8skubeadmcontrolplane_controller.go:480`)
- `(*Kany8sKubeadmControlPlaneReconciler).reconcileClusterKubeconfigSecret(...)` (`internal/controller/controlplane/kany8skubeadmcontrolplane_controller.go:545`)
- `(*Kany8sKubeadmControlPlaneReconciler).SetupWithManager(...)` (`internal/controller/controlplane/kany8skubeadmcontrolplane_controller.go:689`)

Core behavior (high-level):

- Resolve owner `Cluster` via OwnerReferences (`util.GetOwnerCluster`, called from `Reconcile`, `internal/controller/controlplane/kany8skubeadmcontrolplane_controller.go:92`).
- Ensure initial Cluster certificates exist (CAPI cert utilities) (`internal/controller/controlplane/kany8skubeadmcontrolplane_controller.go:121-136`).
- Read the infrastructure cluster `spec.controlPlaneEndpoint` and copy it into `Kany8sKubeadmControlPlane.spec.controlPlaneEndpoint` (`internal/controller/controlplane/kany8skubeadmcontrolplane_controller.go:149-169`).
- Ensure `<cluster>-kubeconfig` Secret exists and matches the expected server endpoint (`reconcileClusterKubeconfigSecret`, `internal/controller/controlplane/kany8skubeadmcontrolplane_controller.go:545`).
- Create/patch a `KubeadmConfig` for the first control plane machine (name convention: `<cluster>-control-plane-0`) (`reconcileInitialControlPlaneKubeadmConfig`, `internal/controller/controlplane/kany8skubeadmcontrolplane_controller.go:332`).
- Generate an infra machine from the provider template and create it (`reconcileInfraMachine`, `internal/controller/controlplane/kany8skubeadmcontrolplane_controller.go:416`).
- Create/patch a CAPI `Machine` pointing at the bootstrap and infra refs (`reconcileControlPlaneMachine`, `internal/controller/controlplane/kany8skubeadmcontrolplane_controller.go:480`).
- Mark initialization based on Cluster `RemoteConnectionProbe=True` or a Ready control plane machine (`reconcileReadiness`, `internal/controller/controlplane/kany8skubeadmcontrolplane_controller.go:195`).

## Internal Helper Packages

### `internal/kro`

- `ResolveInstanceGVK(...)` (`internal/kro/gvk.go:15`, file 61 lines)
  - Reads a `kro.run/v1alpha1 ResourceGraphDefinition` and extracts the instance schema GVK from `spec.schema.{apiVersion,kind}`.
- `ReadInstanceStatus(...)` (`internal/kro/status.go:16`, file 56 lines)
  - Reads normalized fields: `status.ready`, `status.endpoint`, `status.reason`, `status.message`.

### `internal/endpoint`

- `Parse(raw string)` (`internal/endpoint/parse.go:13`, file 77 lines)
  - Parses and validates endpoint strings; enforces `https` scheme, no userinfo/query/fragment/path.
  - Defaults port to 443 if omitted.
  - Defensive error formatting avoids leaking raw URLs (verified by `internal/endpoint/parse_test.go:120`).

### `internal/kubeconfig`

- `SecretName(clusterName)` (`internal/kubeconfig/secret.go:18`, file 54 lines)
- `NewSecret(clusterName, namespace, kubeconfig)` (`internal/kubeconfig/secret.go:26`)
  - Produces a CAPI-compatible `<cluster>-kubeconfig` Secret with type `cluster.x-k8s.io/secret` and label `cluster.x-k8s.io/cluster-name`.

### `internal/dynamicwatch`

File: `internal/dynamicwatch/watcher.go` (265 lines)

Major functions:

- `New(...)` (`internal/dynamicwatch/watcher.go:51`)
- `(*Watcher).Start(...)` (`internal/dynamicwatch/watcher.go:66`)
- `(*Watcher).EnsureWatch(...)` (`internal/dynamicwatch/watcher.go:88`)
- `(*Watcher).enqueue(...)` (`internal/dynamicwatch/watcher.go:134`)

Purpose:

- Create informers dynamically for unstructured resources (e.g. kro instance kinds) and forward events into a controller-runtime channel.
- Coalesce events when the channel is full (with a Prometheus counter `kany8s_dynamicwatch_channel_full_total`, `internal/dynamicwatch/watcher.go:21-28`).

### `internal/constants`

- Requeue intervals:
  - `ControlPlaneNotReadyRequeueAfter` (`internal/constants/constants.go:5`, file 8 lines)
  - `InfrastructureNotReadyRequeueAfter` (`internal/constants/constants.go:7`)

## Acceptance Scripts (Shell)

### kro demo flow

File: `hack/acceptance-test-kro-reflection.sh`

Major shell functions:

- `need_cmd()` (`hack/acceptance-test-kro-reflection.sh:43`)
- `k()` (`hack/acceptance-test-kro-reflection.sh:52`)
- `collect_diagnostics()` (`hack/acceptance-test-kro-reflection.sh:69`)
- `on_exit()` (`hack/acceptance-test-kro-reflection.sh:118`)

Flow summary:

- Create kind cluster, install kro, apply demo RGD (`test/acceptance_test/manifests/kro/rgd.yaml`), install Kany8s CRDs, build/load controller image, deploy, apply `Kany8sControlPlane`, and verify endpoint/initialized contract.

### kro infra reflection (infrastructure)

File: `hack/acceptance-test-kro-infra-reflection.sh`

Flow summary:

- Create kind cluster, install kro, apply infra RGD (`test/acceptance_test/manifests/kro/infra/rgd.yaml`), install/deploy Kany8s, apply `Kany8sCluster`, and verify the provisioned/Ready contract and kro instance spec injection.

### kro infra cluster identity (clusterUID / ownerReferences)

File: `hack/acceptance-test-kro-infra-cluster-identity.sh`

Flow summary:

- Create kind cluster, install Cluster API core + kro, apply ownerRef infra RGD (`test/acceptance_test/manifests/kro/infra/rgd-ownerref.yaml`), install/deploy Kany8s, apply the `Cluster` + `Kany8sCluster` template (`test/acceptance_test/manifests/kro/infra/cluster.yaml.tpl`), and verify `clusterUID` injection plus owned resource `ownerReferences`/label propagation.
- Wrapper runner script: `test/acceptance_test/run-acceptance-kro-infra-cluster-identity.sh`.

### kro demo flow (multi RGD / multi instance kind)

File: `hack/acceptance-test-kro-reflection-multi-rgd.sh`

Flow summary:

- Same shape as the kro demo flow, but applies two RGDs (`demo-control-plane.kro.run` + `demo-control-plane-alt.kro.run`) and creates two `Kany8sControlPlane` objects, proving a single `Kany8sControlPlane` kind can drive multiple kro instance kinds via `spec.resourceGraphDefinitionRef`.
- Wrapper runner script: `test/acceptance_test/run-acceptance-kro-reflection-multi-rgd.sh`.
- Make target: `make test-acceptance-kro-reflection-multi-rgd`.

### self-managed (CAPD + kubeadm)

File: `hack/acceptance-test-capd-kubeadm.sh`

Major shell functions:

- `wait_cluster_condition_with_progress()` (`hack/acceptance-test-capd-kubeadm.sh:121`)
- `collect_diagnostics()` (`hack/acceptance-test-capd-kubeadm.sh:248`)

Flow summary:

- Create kind management cluster with docker.sock mounted, build/load controller image, build a clusterctl-style components bundle from `dist/install.yaml`, run `clusterctl init` with CAPD + CABPK + Kany8s, render + apply `test/acceptance_test/manifests/self-managed-docker/cluster.yaml.tpl` (equivalent to `examples/self-managed-docker/cluster.yaml`), wait for Cluster conditions, and fetch workload kubeconfig.

## Tests

### Unit/envtest

- Driven by `make test` (`Makefile:60-63`), uses envtest assets.
- Most reconciliation behavior is validated via controller-level tests under:
  - `internal/controller/*_test.go`
  - `internal/controller/controlplane/*_test.go`
  - `internal/controller/infrastructure/*_test.go`

### E2E (Ginkgo)

- Suite entrypoint: `TestE2E` (`test/e2e/e2e_suite_test.go:45`, file 102 lines)
- Scenario tests: `test/e2e/e2e_test.go` (338 lines)
  - Token helper: `serviceAccountToken()` (`test/e2e/e2e_test.go:286`)
  - Metrics fetch: `getMetricsOutput()` (`test/e2e/e2e_test.go:325`)
- Helpers: `test/utils/utils.go` (227 lines)
  - `Run(...)` (`test/utils/utils.go:43`)
  - `InstallCertManager()` (`test/utils/utils.go:85`)
  - `LoadImageToKindClusterWithName(...)` (`test/utils/utils.go:137`)

### internal/devtools (repo-policy tests)

The `internal/devtools` package contains tests that enforce repository invariants (docs/examples/config consistency). Example files include:

- `internal/devtools/readme_test.go`
- `internal/devtools/ci_test.go`
- `internal/devtools/rbac_role_test.go`
- `internal/devtools/acceptance_test_script_test.go`

These are useful for preventing drift between documentation, samples, and controller behavior.

## Generated Manifests / Artifacts

Do not edit by hand:

- `config/crd/bases/*.yaml` (generated by `make manifests`)
- `config/rbac/role.yaml` (generated by `make manifests`)
- `dist/install.yaml` (generated by `make build-installer`)
- `**/zz_generated.deepcopy.go` (generated by `make generate`)

## Size / LOC Metrics

Go code (tracked `.go` files):

- Go files: 83
- Total lines: 13,349
- Non-test Go files: 22 files / 3,841 lines
- Test Go files: 61 files / 9,508 lines

Largest Go files (top 10 by lines):

1. `internal/controller/controlplane/kany8skubeadmcontrolplane_reconciler_test.go` (1,516)
2. `internal/controller/kany8scontrolplane_kroinstance_test.go` (1,400)
3. `internal/controller/kany8scontrolplane_kubeconfig_test.go` (1,012)
4. `internal/controller/controlplane/kany8skubeadmcontrolplane_controller.go` (695)
5. `internal/controller/infrastructure/kany8scluster_reconciler_test.go` (686)
6. `internal/controller/kany8scontrolplane_controller.go` (518)
7. `api/v1alpha1/zz_generated.deepcopy.go` (435, generated)
8. `test/e2e/e2e_test.go` (338)
9. `internal/devtools/rbac_role_test.go` (307)
10. `api/infrastructure/v1alpha1/zz_generated.deepcopy.go` (291, generated)
