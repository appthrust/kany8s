# ADR 0011: Extensible ControlPlane Backends (Facade)

- Status: Accepted
- Date: 2026-02-05

## Context

Kany8s currently has two ControlPlane entry points:

- Managed control plane: `Kany8sControlPlane` delegates provisioning to kro (RGD instance), and only consumes the normalized instance status.
- Self-managed (kubeadm): `Kany8sKubeadmControlPlane` implements a kubeadm-based control plane provider.

For self-managed control planes, users may want alternatives to kubeadm (Talos/k0s/RKE2/custom OS/on-prem platforms). Today that typically requires implementing new controller logic in-tree or forking Kany8s.

We want a design that allows users to add self-managed ControlPlane backends out-of-tree while preserving:

- A single CAPI-facing facade (`Kany8sControlPlane`).
- Provider-agnostic boundaries: the facade does not read provider-specific CR shapes; it only consumes a normalized status contract.
- Compatibility with the existing kro-managed (managed) path and the existing kubeadm path.

Related decisions:

- Provider-agnostic via kro: `docs/adr/0001-provider-agnostic-kany8s-via-kro.md`
- Normalized status contract (fields): `docs/adr/0002-normalized-rgd-instance-status-contract.md`
- Kubeconfig Secret strategy: `docs/adr/0004-kubeconfig-secret-strategy.md`
- Dynamic GVK RBAC tradeoffs: `docs/adr/0007-dynamic-gvk-rbac-tradeoffs.md`
- Self-managed kubeadm boundaries: `docs/adr/0009-self-managed-kubeadm-boundaries.md`

## Decision

### 1) `Kany8sControlPlane` is the facade for multiple backends

`Kany8sControlPlane` remains the only ControlPlane object referenced by CAPI `Cluster.spec.controlPlaneRef`.

`Kany8sControlPlane` delegates actual provisioning to a selected backend and only reflects:

- endpoint
- initialization
- conditions / failure* (terminal errors only)
- optional kubeconfig Secret reconciliation

Backends are:

- kro-backed backend (current behavior): a kro instance created/owned 1:1 by `Kany8sControlPlane`.
- kubeadm delegate backend (builtin): an internal `Kany8sKubeadmControlPlane` created/owned by the facade.
- external backend (extensible): a user-defined CRD/controller created/owned 1:1 by the facade.

Important notes:

- The facade pattern is about a single CAPI-facing entry point. It does not require all backend implementations to be identical; however, the facade must be able to consume backend readiness via a stable, provider-agnostic status contract.
- All backend objects referenced/created by the facade are assumed to be namespaced. Cluster-scoped backends are out of scope (see "External backend constraints").

### 2) Backend status contract is the existing normalized ControlPlane contract

All backends MUST expose the same normalized status fields so the facade can stay provider-agnostic.

The required/optional fields and endpoint format are the same as ADR 0002:

- Required:
  - `status.ready: boolean`
  - `status.endpoint: string` (`https://host[:port]` or `host[:port]`; port defaults to 443)
- Optional:
  - `status.reason: string`
  - `status.message: string`
  - `status.kubeconfigSecretRef` (`name`, optional `namespace`)

Additional recommended fields (non-breaking, optional):

- `status.observedGeneration: int64`
  - Meaning: the backend controller has observed and acted on the current `metadata.generation` of the backend object.
  - Why: improves debuggability and upgrade/topology convergence when the facade updates backend spec.
- `status.terminal: boolean`
  - Meaning: the backend has encountered an unrecoverable failure for the current desired state.
  - Why: allows backend-authored terminal errors to be surfaced consistently via the ControlPlane provider contract (`failureReason`/`failureMessage`).

Notes:

- `status.ready` means "ControlPlane ready" (at minimum, endpoint is known).
- The facade MUST tolerate missing fields as described in ADR 0002.

### 3) Kubeconfig Secret responsibility follows ADR 0004 (Option B default)

The facade continues the default behavior from ADR 0004 Option B:

- If a backend sets `status.kubeconfigSecretRef`, Kany8s reads the source Secret `data.value` and writes/updates the CAPI-compatible `<cluster>-kubeconfig` Secret.

Security/namespace constraints:

- By default, the kubeconfig source Secret is expected to be in the same namespace as the `Kany8sControlPlane`.
- If `status.kubeconfigSecretRef.namespace` is set to a different namespace, installations MUST explicitly opt in by extending RBAC and (recommended) configuring an allowlist. Otherwise, the facade SHOULD treat it as an error and avoid reading cross-namespace Secrets.

Backends MAY instead implement ADR 0004 Option A:

- If `status.kubeconfigSecretRef` is not set, the backend may create/maintain `<cluster>-kubeconfig` directly.

### 4) Ownership/lifecycle: 1:1 backend object owned by the facade

To keep UX consistent (apply `Cluster` + `Kany8sControlPlane` only) and to make ClusterClass/Topology viable, the facade owns exactly one backend object.

- Backend object name/namespace is the same as the `Kany8sControlPlane` (1:1).
- The facade sets an OwnerReference to `Kany8sControlPlane` so deletion is handled by GC.

External backend constraints:

- Backend objects MUST be namespaced and MUST live in the same namespace as the facade object.
  - Rationale: namespaced OwnerReferences (facade -> backend) and the current dynamic watch strategy are namespaced.
  - Cluster-scoped backends cannot be owned by a namespaced facade object and require a different watch/enqueue model.

### 5) External backend creation uses a minimal spec injection contract

Because the facade must create the backend object without knowing provider-specific spec shapes, we standardize a tiny "spec injection" contract:

- External backend `.spec` is supplied as an arbitrary JSON object (`backendSpec`).
- The facade injects and enforces `.spec.version` (overwriting user-supplied values), same rationale as ADR 0003.

Optional (recommended) identity injection for backends that need to set OwnerReferences/labels to the owner CAPI `Cluster`:

- `.spec.clusterName`
- `.spec.clusterNamespace`
- `.spec.clusterUID`

Backends that opt into identity injection MUST include these fields in their CRD schema.

The facade does not read backend spec fields.

Implementation guidance (so this stays extensible in practice):

- The facade SHOULD update backend objects using a merge strategy (server-side apply or patch) and only force ownership of the injected keys (`spec.version`, identity keys) to avoid clobbering backend-side defaulting.
- Backend controllers SHOULD treat injected fields as authoritative and MUST NOT mutate them.
- The facade SHOULD also apply `metadata.labels["cluster.x-k8s.io/cluster-name"]=<cluster-name>` on backend objects for CAPI ecosystem compatibility.

### 6) Dynamic watch and RBAC

The facade watches backend objects so status changes trigger reconciliation.

- kro backend: reuse existing dynamic watch machinery for kro instances.
- external backend: watch the backend GVK dynamically (same pattern).

Watch resolution constraints and mitigations:

- The controller SHOULD resolve GVK -> GVR using discovery/RESTMapper. Relying on kind-to-plural guessing is not robust for CRDs with irregular pluralization.
- EnsureWatch SHOULD be non-blocking (or have a short timeout) and surface failures via Conditions so reconciliation does not deadlock when:
  - the backend CRD is not installed yet
  - RBAC for the backend resource is missing
  - discovery is temporarily unavailable

RBAC:

- kro backend retains the MVP tradeoff in ADR 0007 (`kro.run resources=*`).
- external backends require install-time RBAC extension by the operator for the specific backend GVK.
  - Kany8s will not ship cluster-wide `*.*` wildcard RBAC.

If cross-namespace kubeconfig source Secrets are allowed, RBAC must also be extended accordingly (recommended: keep it disabled by default).

## API / UX (Proposed shape)

This ADR fixes the direction and contracts; exact field names can be refined, but the facade must be able to select exactly one backend.

Suggested v1alpha1-compatible shape (backward compatible for existing kro users):

- Keep existing kro fields but make them optional:
  - `spec.resourceGraphDefinitionRef` (kro backend selector)
  - `spec.kroSpec`
- Add exactly one of:
  - `spec.kubeadm` (builtin delegate backend)
  - `spec.externalBackend` (extensible backend)

Validation requirements:

- Exactly one backend selector MUST be set (kro vs kubeadm vs external).
- Backend selector fields MUST be immutable after creation (changing backend type is treated as replace-the-control-plane, not an in-place update).

`spec.externalBackend` carries:

- `apiVersion` / `kind` (backend GVK)
- `spec` (arbitrary JSON object passed through to backend `.spec`)

Optional (recommended) for robust dynamic watch:

- `resource` (plural resource name) to avoid kind-to-resource guessing pitfalls when the controller cannot resolve the GVR via discovery.

`Kany8sControlPlaneTemplate` must mirror the same backend selection input surface.

### Examples (shape only)

Existing kro backend (no change in intent):

```yaml
apiVersion: controlplane.cluster.x-k8s.io/v1alpha1
kind: Kany8sControlPlane
metadata:
  name: demo
  namespace: default
spec:
  version: v1.34.0
  resourceGraphDefinitionRef:
    name: eks-control-plane.kro.run
  kroSpec:
    region: ap-northeast-1
```

External backend (created 1:1; backend controller is out-of-tree):

```yaml
apiVersion: controlplane.cluster.x-k8s.io/v1alpha1
kind: Kany8sControlPlane
metadata:
  name: demo
  namespace: default
spec:
  version: v1.34.0
  externalBackend:
    apiVersion: controlplane.example.com/v1alpha1
    kind: ExampleSelfManagedControlPlane
    spec:
      # backend-specific inputs (arbitrary JSON object)
      datacenter: dc-a
      # spec.version / spec.cluster* are injected/overwritten by the facade.
```

## Backend authoring notes (external backends)

To integrate with the facade, backend authors should implement:

- Input (spec): accept `spec.version` and (optionally) `spec.clusterName`, `spec.clusterNamespace`, `spec.clusterUID`.
- Output (status): implement the normalized ControlPlane status contract (ADR 0002 fields):
  - `status.ready`, `status.endpoint` (required)
  - `status.reason`, `status.message`, `status.kubeconfigSecretRef` (optional)

Recommended additions:

- Set `status.observedGeneration` to the backend object's `metadata.generation` once the new desired state has been observed.
- Set `status.terminal=true` for unrecoverable failures, and provide `status.reason`/`status.message` suitable for surfacing.
- Kubeconfig:
  - Option B (recommended): write a provider-specific kubeconfig Secret and set `status.kubeconfigSecretRef`.
  - Option A: create/maintain `<cluster>-kubeconfig` directly (do not set `kubeconfigSecretRef`).

## Consequences

- Users can implement self-managed control plane backends out-of-tree and plug them in without forking Kany8s.
- `Kany8sControlPlane` stays provider-agnostic at read time by consuming only the normalized status contract.
- Backends must implement both:
  - normalized status output (ADR 0002 fields)
  - minimal spec injection inputs (`spec.version`, and optionally cluster identity)
- Installations that use external backends must explicitly extend RBAC for the backend GVK.

## Compatibility / Migration Notes

- This ADR defines the direction for a facade-based extensibility model. Existing `Kany8sKubeadmControlPlane` users remain supported.
- If/when `spec.kubeadm` is introduced on the facade, the implementation MUST provide an adapter that projects kubeadm readiness into the normalized status contract (for example by translating `Ready/Creating` Conditions and the resolved endpoint), so the facade can remain status-contract driven.
