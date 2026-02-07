# ADR 0010: CAPD Facade Behind the Kany8s Suite

- Status: Superseded
- Date: 2026-02-05

## Context

Users may want a local/dev backend where they can apply only:

- `Cluster.spec.infrastructureRef = Kany8sCluster`
- `Cluster.spec.controlPlaneRef = Kany8sControlPlane`

and still reach `Cluster Available=True` on CAPD (Docker) without manually applying `DockerCluster`/`DockerMachineTemplate` or a separate kubeadm control plane object.

This ADR is superseded by `docs/adr/0011-extensible-controlplane-backends.md`, which generalizes the "facade delegates to a backend" pattern.

## Proposal (Decision)

Make CAPD a backend implementation detail behind the Kany8s suite:

- Infra: `Kany8sCluster` uses kro mode with a CAPD-specific infra RGD.
- ControlPlane: `Kany8sControlPlane` gains a kubeadm delegate backend that creates/patches an internal `Kany8sKubeadmControlPlane`.

## Consequences

- Preserves provider-agnostic principle for infra (CAPD specifics live in RGD).
- Introduces a second backend mode for `Kany8sControlPlane`.
- Requires clear internal vs user-facing object documentation.

## Alternatives considered

- Embed CAPD logic directly in Kany8s controllers (rejected: provider-specific branching).
- Nested CAPI (kro creates an inner `Cluster`) (rejected: confusing UX and ownership).
