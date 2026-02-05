# ADR 0006: Topology Version and Template API Groups

- Status: Accepted
- Date: 2026-02-05

## Context

Kany8s is designed to be used via ClusterClass/Topology (Cluster API). That introduces two important contracts:

1) How Kubernetes version is specified and propagated.
2) Which API group owns which template kinds.

## Decision

### Version propagation

- `Cluster.spec.topology.version` is the single source of truth for Kubernetes version.
- Generated `Kany8sControlPlane.spec.version` must match `Cluster.spec.topology.version`.
- Kany8s injects `Kany8sControlPlane.spec.version` into the referenced kro instance `.spec.version`.

### Template API groups

We align templates with their owning API group:

- `Kany8sClusterTemplate` is owned by `infrastructure.cluster.x-k8s.io`.
- A historical `Kany8sClusterTemplate` under `controlplane.cluster.x-k8s.io` existed temporarily, but it is removed.

Keywords (for clarity and repo policy):

- Kany8sClusterTemplate
- infrastructure.cluster.x-k8s.io
- controlplane.cluster.x-k8s.io
- removed

## Consequences

- Version drift is minimized by having exactly one user-facing version source.
- Topology consumers can reason about templates without API-group confusion.
