# ADR 0003: kro Instance Lifecycle and Spec Injection

- Status: Accepted
- Date: 2026-02-05

## Context

Kany8s must create and reconcile kro instances whose GVK is not known at compile time:

- The user selects an RGD by name.
- kro generates a CustomResourceDefinition (CRD) for the instance kind.
- The instance GVK is derived from `ResourceGraphDefinition.spec.schema.{apiVersion,kind}`.

Kany8s must also ensure certain spec fields are always enforced (for example the Kubernetes version).

## Decision

### ControlPlane (`Kany8sControlPlane` -> kro instance)

- `Kany8sControlPlane.spec.resourceGraphDefinitionRef.name` selects an RGD.
- Kany8s resolves the instance GVK dynamically from the referenced RGD.
- Kany8s creates exactly one kro instance per control plane (1:1):
  - instance `metadata.name` == control plane name
  - instance `metadata.namespace` == control plane namespace
- Kany8s writes the kro instance `.spec` by:
  1) parsing `Kany8sControlPlane.spec.kroSpec`
  2) injecting `spec.version` into the instance `.spec` (overwriting any user-provided value)

Important contract:

- `Kany8sControlPlane` assumes `spec.kroSpec` is a JSON object because the controller injects `spec.version` into the kro instance `.spec`.
- Therefore: `spec.kroSpec` must be a JSON object.

### Infrastructure (`Kany8sCluster` -> kro instance)

- When `Kany8sCluster.spec.resourceGraphDefinitionRef` is set, Kany8sCluster runs in “kro mode”.
- Kany8s resolves the instance GVK dynamically from the referenced RGD.
- Kany8s creates a 1:1 instance (name/namespace == `Kany8sCluster`).
- Kany8s injects cluster identity into the instance spec:
  - `spec.clusterName` (always)
  - `spec.clusterNamespace` (always)
  - `spec.clusterUID` (only when the referenced RGD schema declares `clusterUID`)

## Consequences

- Controllers can stay generic by using `unstructured.Unstructured` for instances.
- RGD authors can rely on `spec.version` being enforced by Kany8s for control planes.
- For infra graphs that need owner references (provider ownership patterns), `clusterUID` injection is available as an opt-in.
