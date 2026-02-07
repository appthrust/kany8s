# Acceptance Criteria

This document defines acceptance criteria for a repository state where the product requirements in `docs/PRD.md` are implemented.

The criteria below are intentionally written as verifiable checks (commands + expected outcomes).

## 1. Global (always required)

- `make test` succeeds (unit/envtest).
- `make test-e2e` succeeds (smoke e2e).
- `make test-acceptance-kro-reflection` succeeds (kro demo flow).
  - Legacy alias: `make test-acceptance`.
- `make test-acceptance-kro-infra-reflection` succeeds (kro infra reflection flow).
- `make build` succeeds.
- `make docker-build IMG=example.com/kany8s/controller:acceptance` succeeds.

## 2. Managed Control Plane (kro RGD mode)

Given:

- A fresh kind management cluster.
- kro is installed and the demo RGD is accepted (`examples/kro/ready-endpoint/rgd.yaml`).
- Kany8s is installed (CRDs + controller-manager).

Acceptance:

- Applying a `controlplane.cluster.x-k8s.io/v1alpha1` `Kany8sControlPlane` that references the accepted RGD results in:
  - A 1:1 kro instance resource (same name/namespace as `Kany8sControlPlane`).
  - `Kany8sControlPlane.spec.controlPlaneEndpoint.{host,port}` becomes non-empty/non-zero.
  - `Kany8sControlPlane.status.initialization.controlPlaneInitialized=true`.
  - `Kany8sControlPlane` reaches `Ready=True`.

When Cluster API v1beta2 controllers are installed and a `Cluster` references `Kany8sControlPlane` via `spec.controlPlaneRef`:

- `Cluster.spec.controlPlaneEndpoint` is mirrored from `Kany8sControlPlane.spec.controlPlaneEndpoint` (per the CAPI contract).

## 3. Kubeconfig Secret (managed mode)

When the kro instance exposes `status.kubeconfigSecretRef`:

- Kany8s creates/updates the target Secret `<cluster>-kubeconfig` with:
  - `type=cluster.x-k8s.io/secret`
  - label `cluster.x-k8s.io/cluster-name=<cluster>`
  - `data["value"]` containing a valid kubeconfig.
- If the source Secret changes, the target Secret follows.
- If the source Secret is missing / missing `data["value"]` / contains invalid kubeconfig:
  - `KubeconfigSecretReconciled=False` is surfaced via conditions (and does not stay stale).
  - Kany8s does not overwrite an existing valid target kubeconfig with invalid bytes.

## 4. Infrastructure Provider (stub)

Acceptance for the minimal InfrastructureCluster implementation:

- Creating `infrastructure.cluster.x-k8s.io/v1alpha1` `Kany8sCluster` results in:
  - `status.initialization.provisioned=true`.
  - `Ready=True`.
  - `status.failureReason/status.failureMessage` are empty (not set during normal operation).

## 5. ClusterClass / Topology

Given a management cluster with CAPI v1beta2 contract enabled:

- `examples/capi/clusterclass.yaml` can be applied.
- Creating a `Cluster` via `spec.topology` results in:
  - `Kany8sControlPlane` created from the template.
  - Topology version changes propagate:
    - `Cluster.spec.topology.version` -> `Kany8sControlPlane.spec.version` -> kro instance `spec.version`.

## 6. Self-Managed Control Plane (CAPD + kubeadm)

Given:

- A fresh kind management cluster.
- Cluster API core + CABPK + CAPD installed.
- Kany8s installed (with the self-managed control plane provider API/controller).

Acceptance:

- The repo contains a runnable CAPD+kubeadm example (e.g. `examples/self-managed-docker/`).
- Applying the example results in a real workload cluster with a functional API server:
  - `Cluster` reaches:
    - `RemoteConnectionProbe=True`
    - `Available=True`
  - `Cluster.spec.controlPlaneEndpoint` is populated and stable.
  - `<cluster>-kubeconfig` Secret exists and is usable to connect to the workload cluster.
    - `kubectl --kubeconfig <decoded-secret> get nodes` succeeds.
    - At least 1 Node exists and is `Ready=True` (NoWorkers is allowed).

Endpoint correctness:

- The control plane endpoint source of truth is the infrastructure provider endpoint (CAPD).
  - Kany8s does not overwrite infra-provided endpoints with guessed values.

Certificates:

- Cluster certificates required for kubeadm bootstrap exist as CAPI cluster secrets in the management cluster namespace (e.g. `<cluster>-ca`, `<cluster>-sa`, `<cluster>-proxy`, `<cluster>-etcd`) and are stable/idempotent.

## 7. Acceptance Automation

- `make test-acceptance-kro-reflection` remains the supported automated check for the kro demo flow.
  - Legacy alias: `make test-acceptance`.
- A dedicated acceptance script/target exists for self-managed provisioning (CAPD + kubeadm) and passes on a fresh kind cluster:
  - Example: `make test-acceptance-capd-kubeadm`
  - Legacy alias: `make test-acceptance-self-managed`

## 8. Security / Safety

- Kany8s never logs kubeconfig contents.
- Kany8s avoids logging raw endpoints when they might contain credentials (sanitized messages).
