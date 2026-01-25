# RGD Contract

This document defines the minimal contract an RGD instance (a custom resource created by kro from a `ResourceGraphDefinition`) MUST expose so `Kany8sControlPlane` can consume it without provider-specific logic.

## Status Fields (Normalized)

Kany8s reads the following fields from the RGD instance `status`.

- `status.ready` (required, boolean)
  - Meaning: "the managed control plane is Ready" (control-plane ready, not addons ready).
  - MUST always be present as a boolean (avoid missing-field pitfalls).

- `status.endpoint` (required, string)
  - Meaning: the control plane API endpoint.
  - Accepted formats: `https://host[:port]` or `host[:port]`.
  - If `port` is omitted, Kany8s assumes `443`.
  - SHOULD be non-empty when `status.ready=true`. It MAY be empty while provisioning.

- `status.reason` (optional, string)
  - Short machine-friendly reason for the current state.

- `status.message` (optional, string)
  - Human-friendly message describing the current state.

### Reserved Status Fields

kro reserves `status.conditions` and `status.state`. Do not use them for the above contract; use the dedicated fields listed here.

## Semantics

- Kany8s treats `status.ready` and `status.endpoint` as the source of truth for:
  - `Kany8sControlPlane.spec.controlPlaneEndpoint`
  - `Kany8sControlPlane.status.initialization.controlPlaneInitialized`
  - `Kany8sControlPlane.status.conditions`
  - `Kany8sControlPlane.status.failureReason` / `Kany8sControlPlane.status.failureMessage`

If `status.ready=false`, Kany8s may surface `status.reason`/`status.message` as failure details.

## Example

```yaml
apiVersion: kro.run/v1alpha1
kind: EKSControlPlane
metadata:
  name: demo-cluster
  namespace: default
status:
  ready: true
  endpoint: https://demo.eks.example.com:6443
  reason: ControlPlaneReady
  message: Control plane is ACTIVE and endpoint is available
```
