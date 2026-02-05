# ADR 0002: Normalized RGD Instance Status Contract

- Status: Accepted
- Date: 2026-02-05

## Context

To keep Kany8s provider-agnostic, Kany8s controllers must make decisions without reading provider-specific resources.
The only supported input is the kro instance `status`.

Also, kro has reserved status fields (`status.conditions`, `status.state`) and has known quirks around field materialization, so the contract must be both strict (for RGD authors) and defensively consumable (for controllers).

## Decision

### ControlPlane contract (RGD instance backing `Kany8sControlPlane`)

RGD instances MUST expose these top-level status fields:

- `status.ready: boolean` (required)
  - Meaning: “ControlPlane ready” (at minimum, the API endpoint is known).
- `status.endpoint: string` (required)
  - Accepted formats: `https://host[:port]` or `host[:port]`.
  - If port is omitted, Kany8s treats it as `443`.

RGD instances SHOULD expose:

- `status.reason: string` (optional)
- `status.message: string` (optional)
- `status.kubeconfigSecretRef: object` (optional)
  - Meaning: reference to a provider-specific source Secret that contains a kubeconfig at `data.value`.
  - Intended shape:
    - `name: string`
    - `namespace: string` (optional; if omitted, the instance namespace is assumed)

Controllers MUST tolerate missing fields safely:

- Missing `status.ready` is treated as `false`.
- Missing `status.endpoint` is treated as empty.

This tolerance exists to keep reconciliation safe even if kro does not materialize a field as expected.

### Parent RGD (infra + control plane)

If the RGD referenced by `Kany8sControlPlane` composes infrastructure resources in addition to a control plane,
it MUST project the child control plane contract into the parent's top-level `status`.

Projection guidance (best-effort forwarding):

- `status.ready` <- `controlPlane.status.ready` (required)
- `status.endpoint` <- `controlPlane.status.endpoint` (required)
- `status.reason` <- `controlPlane.status.reason` (optional)
- `status.message` <- `controlPlane.status.message` (optional)
- `status.kubeconfigSecretRef` <- `controlPlane.status.kubeconfigSecretRef` (optional)

Kany8s does not read nested `controlPlane.status.*` directly.

### Infrastructure contract (RGD instance backing `Kany8sCluster` in kro mode)

RGD instances MUST expose:

- `status.ready: boolean` (required)
  - Meaning: “Infrastructure ready / provisioned”.

RGD instances MAY expose:

- `status.reason: string` (optional)
- `status.message: string` (optional)
- `status.endpoint: string` (optional; reserved for future use cases where infra needs to surface an endpoint)

### Reserved status fields

kro reserves:

- `status.conditions`
- `status.state`

Do not use them to satisfy the Kany8s contract.

## Consequences

- RGD authors have a clear “output surface” to target.
- Kany8s controllers can implement one status-consumption path for all providers.
- Parent-RGD patterns remain compatible with the provider-agnostic boundary.
