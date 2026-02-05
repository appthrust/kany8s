# ADR 0001: Provider-Agnostic Kany8s via kro

- Status: Accepted
- Date: 2026-02-05

## Context

Kany8s aims to be a Cluster API (CAPI) provider suite that can drive managed control planes (EKS/GKE/AKS, etc.) without embedding provider-specific logic in the controllers.

Traditional providers implement per-cloud controllers (CAPA/CAPZ/CAPG, ...). That model scales feature-wise, but it scales poorly when the core differentiator is “add providers cheaply”.

kro (ResourceGraphDefinition / RGD) provides a Kubernetes-native composition engine (DAG + status projection). That makes it a good fit for keeping “provider-specific realization” outside Kany8s.

## Decision

1) Kany8s controllers remain provider-agnostic.

- Kany8s does not read provider-specific CRDs (ACK, ASO, Config Connector, etc.) directly.
- Kany8s reads only a kro instance's normalized `status`.

2) Provider-specific behavior lives in RGD(s) and the underlying provider controllers.

- RGD(s) create/compose provider resources.
- RGD(s) project provider-specific statuses into a normalized instance status contract.

## Consequences

- We must define a strict, minimal status contract for RGD instances.
  - See `docs/adr/0002-normalized-rgd-instance-status-contract.md`.
- Kany8s needs to manage kro instances dynamically (unknown GVK at build time).
  - See `docs/adr/0003-kro-instance-lifecycle-and-spec-injection.md`.
- RBAC for kro instances may be broader than ideal in the MVP due to dynamic GVK.
  - See `docs/adr/0007-dynamic-gvk-rbac-tradeoffs.md`.

## Alternatives considered

- Put provider-specific logic in Kany8s controllers.
  - Rejected: becomes “if EKS then ... else if GKE ...”; provider additions require code changes.
- Adopt CAPT's “Template -> Apply” as the core orchestration model.
  - Rejected: kro already provides graph orchestration; reintroducing an Apply-unit abstraction increases operational complexity.
