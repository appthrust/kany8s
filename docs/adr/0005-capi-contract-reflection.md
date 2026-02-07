# ADR 0005: CAPI Contract Reflection

- Status: Accepted
- Date: 2026-02-05

## Context

Cluster API defines contracts for ControlPlane and Infrastructure providers. For Kany8s to integrate cleanly with CAPI, it must populate the expected fields and conditions.

In particular:

- ControlPlane providers are expected to surface the API endpoint and initialization readiness.
- Infrastructure providers are expected to surface “provisioned” readiness for infra.

## Decision

### ControlPlane contract

- `Kany8sControlPlane` is the endpoint provider.
- Kany8s sets `Kany8sControlPlane.spec.controlPlaneEndpoint` by parsing the kro instance `status.endpoint`.
- Kany8s sets `Kany8sControlPlane.status.initialization.controlPlaneInitialized` when the endpoint is known and valid.
- Kany8s reports progress primarily via `status.conditions`.

Kany8s does not patch `Cluster.spec.controlPlaneEndpoint` directly. The CAPI Cluster controller mirrors the endpoint from the referenced ControlPlane object per the contract.

### Infrastructure contract

- `Kany8sCluster.status.initialization.provisioned` reflects infra readiness.
- In stub mode, `provisioned=true` is used to unblock CAPI flows.
- In kro mode, `provisioned` mirrors the infra RGD instance `status.ready`.

## Consequences

- Kany8s stays aligned with CAPI's expected behavior (mirroring, conditions, and readiness computation).
