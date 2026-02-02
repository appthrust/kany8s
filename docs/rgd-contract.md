# RGD Contract

This document defines the minimal contract an RGD instance (a custom resource created by kro from a `ResourceGraphDefinition`) MUST expose so `Kany8sControlPlane` (and `Kany8sCluster` in kro mode) can consume it without provider-specific logic.

## Status Fields (Normalized)

Kany8s reads the following fields from the RGD instance `status`.

Note:

- For RGD authors: treat `status.ready` and `status.endpoint` as required outputs of your RGD instance.
- Runtime behavior: the Kany8s controller tolerates missing fields by treating them as "not ready" (`ready=false`, empty endpoint) so it can keep reconciling safely even if kro status materialization is imperfect.
  - See `docs/rgd-guidelines.md` for pitfalls and recommended patterns.

### ControlPlane (for `Kany8sControlPlane`)

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

- `status.kubeconfigSecretRef` (optional, object)
  - Meaning: reference to a provider-specific "source" Secret that contains a kubeconfig in `data.value`.
  - If set, Kany8s copies `data.value` into the CAPI-compatible `<cluster>-kubeconfig` Secret and reports progress via the `KubeconfigSecretReconciled` Condition.

### Parent RGD (infra + control plane)

Some environments want the RGD referenced by `Kany8sControlPlane` to also compose infrastructure resources.
In this "Parent RGD" approach (infra + control plane), Kany8s still reads only the instance it created.

Your Parent RGD MUST project the child control plane contract into the parent's top-level `status`.
Kany8s does not read provider-specific child resources, nor nested `controlPlane.status.*` fields directly.

Projection guidance (best-effort forwarding):

- `status.ready` <- `controlPlane.status.ready` (required)
- `status.endpoint` <- `controlPlane.status.endpoint` (required)
- `status.reason` <- `controlPlane.status.reason` (optional)
- `status.message` <- `controlPlane.status.message` (optional)
- `status.kubeconfigSecretRef` <- `controlPlane.status.kubeconfigSecretRef` (optional)

### Infrastructure (for `Kany8sCluster`)

If an RGD instance is intended to back `Kany8sCluster` in kro mode, it MUST expose:

- `status.ready` (required, boolean)
  - Meaning: "the infrastructure is provisioned" (infrastructure ready, not control-plane ready).

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

- Kany8s sets `Ready=True` only when:
  - `status.ready=true` and `status.endpoint` is present, and
  - if `status.kubeconfigSecretRef` is set, the kubeconfig Secret has been reconciled (`KubeconfigSecretReconciled=True`).

Kany8s surfaces `status.reason` / `status.message` primarily via Conditions (e.g., Ready/Creating).
`Kany8sControlPlane.status.failureReason` / `failureMessage` are reserved for terminal, controller-detected errors (for example: invalid `spec.kroSpec`, invalid `status.endpoint`) and are cleared during normal provisioning.

- For `Kany8sCluster` in kro mode:
  - `Kany8sCluster.status.initialization.provisioned` (i.e., `status.initialization.provisioned`) reflects the infrastructure RGD instance `status.ready`.

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
