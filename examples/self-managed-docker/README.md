# self-managed-docker (CAPD + kubeadm)

This example matches the "self-managed" acceptance flow and the manual runbook (`docs/runbooks/capd-kubeadm.md`).

In this repo, "self-managed" means:

- A Kind-based management cluster runs Cluster API (CAPI) + providers.
- That management cluster creates a workload cluster via CAPD (Docker) + CABPK (kubeadm).
- CAPI reports the workload cluster as reachable (`RemoteConnectionProbe=True`) and available (`Available=True`).
- You can fetch a kubeconfig via `clusterctl get kubeconfig` and connect to the workload cluster.

Note: The workload cluster's node may stay `NotReady` if you do not install a CNI plugin.

## What gets applied

`cluster.yaml` is a concrete, manual sample.
The acceptance flow renders and applies `test/acceptance_test/manifests/self-managed-docker/cluster.yaml.tpl` (same shape), which creates these objects in the management cluster:

- `cluster.x-k8s.io/v1beta2, Kind=Cluster`
  - `spec.infrastructureRef` points to a CAPD `DockerCluster`
  - `spec.controlPlaneRef` points to `Kany8sKubeadmControlPlane`
- `infrastructure.cluster.x-k8s.io/v1beta2, Kind=DockerCluster` (CAPD)
- `infrastructure.cluster.x-k8s.io/v1beta2, Kind=DockerMachineTemplate` (CAPD)
- `controlplane.cluster.x-k8s.io/v1alpha1, Kind=Kany8sKubeadmControlPlane` (this project)

This is intentionally *not* a Kro-based flow.
Kro is used in a different acceptance path (see `hack/acceptance-test-kro-reflection.sh`; legacy alias: `hack/acceptance-test.sh`).
In this self-managed example, the infrastructure provider is CAPD, so the CAPD CRs are created directly.

## How to run

Recommended (clean + artifacts + cleanup):

```bash
test/acceptance_test/run-acceptance-capd-kubeadm.sh
```

Or run the underlying target directly:

```bash
make test-acceptance-capd-kubeadm
```

Legacy alias:

```bash
make test-acceptance-self-managed
```

Useful environment variables:

- `KIND_CLUSTER_NAME` (short name recommended)
- `ARTIFACTS_DIR` (default under `/tmp/...`)
- `CLEANUP=true|false` (default: `true`)
- `NAMESPACE` (default: `default`)
- `CLUSTER_NAME` (default: `demo-self-managed-docker`)

## Gotchas

- CAPD needs access to the host Docker daemon.
  The acceptance script mounts `/var/run/docker.sock` into the Kind node.
- Workload Docker containers live on the host Docker daemon (not inside Kind).
  If leftover containers exist for the same `CLUSTER_NAME`, you may see TLS/CA mismatch issues.
  The acceptance runner deletes them by default (see `REUSE_EXISTING_WORKLOAD_DOCKER_CONTAINERS`).
- Node `NotReady` due to missing CNI is expected unless you install a CNI in the workload cluster.
