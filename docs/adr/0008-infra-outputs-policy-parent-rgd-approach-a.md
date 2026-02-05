# ADR 0008: Infra Outputs Policy (Parent RGD / Approach A)

- Status: Accepted
- Date: 2026-02-05

## Context

When both infrastructure and control plane are involved, it is tempting to pass “infra outputs” (VPC IDs, subnet IDs, security groups, etc.) between Kany8s CRDs.

However, generalizing outputs tends to grow an API surface similar to Terraform outputs (Secret-based outputs, synchronization semantics, versioning), which conflicts with Kany8s's “thin provider” goal.

## Decision

We do not introduce “generic outputs” passing between Kany8s CRDs as a core concept.

If infra -> control plane value passing is required, it must stay inside a kro graph:

- Use a Parent RGD that composes infra + control plane.
- Use in-graph references (`network.status.*` -> `controlPlane.spec.*`) to pass values.
- Project the control plane contract to the Parent RGD's top-level status.

This keeps:

- provider-specific details inside RGD(s)
- Kany8s controllers reading only normalized instance `status`

## Alternatives considered

- Typed outputs on `Kany8sCluster.status` (provider-agnostic subset)
  - Rejected: standardization pressure grows quickly; increases API/compat cost.
- Secret/ConfigMap outputs passing
  - Rejected: reintroduces Template->Apply / outputs operational complexity.

## Consequences

- Cross-resource dependencies are expressed and versioned within RGD(s).
- Observability requirements increase for Parent RGDs (must project status).
