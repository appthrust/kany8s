# ADR 0007: dynamic GVK RBAC Tradeoffs

- Status: Accepted
- Date: 2026-02-05

## Context

Kany8s creates/updates kro instances whose GVK is resolved at runtime from the selected RGD.
That is a dynamic GVK problem.

RBAC in Kubernetes is typically declared against known resources (plural names). With dynamic GVK, it's difficult to pre-enumerate all instance kinds.

## Decision

For the MVP, Kany8s grants broad RBAC for kro instances:

- group: `kro.run`
- resources: `resources=*`

This is intentionally documented as a tradeoff.

future tightening approach:

- Allowlist approved RGDs / instance kinds and restrict RBAC to those plural resources.
- Split ClusterRoles by provider catalog entries so installations can opt-in.

## Consequences

- Install-time RBAC is stronger than least-privilege.
- Operational controls (who can create RGDs, who can reference RGDs) become important.
