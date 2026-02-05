# ADR 0009: Self-Managed (kubeadm) Boundaries

- Status: Accepted
- Date: 2026-02-05

## Context

Kany8s supports a self-managed control plane path (kubeadm) primarily for local/dev and as a “working cluster” reference path.

This path must be explicit about boundaries and ownership:

- where the endpoint comes from
- which controller owns kubeconfig/certs
- how to satisfy CAPI readiness conditions

## Decision

- Self-managed control planes are represented by `Kany8sKubeadmControlPlane`.
- endpoint source of truth is the infrastructure provider の `spec.controlPlaneEndpoint`.
  - `Kany8sKubeadmControlPlane` reads the infra endpoint and writes it into `Kany8sKubeadmControlPlane.spec.controlPlaneEndpoint`.
- `<cluster>-kubeconfig` Secret is created/maintained to satisfy the Cluster API contract.
- Certificates follow Cluster API utilities (`util/secret`) naming/format.

## Consequences

- Infra and control plane remain loosely coupled: kubeadm path does not guess endpoints.
- Kubeconfig and certificates remain consistent with Cluster API expectations.
