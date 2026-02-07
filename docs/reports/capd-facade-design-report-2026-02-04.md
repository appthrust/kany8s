# Design Report: CAPD Workload Cluster via `Kany8sCluster`/`Kany8sControlPlane` Facade

- Date: 2026-02-04
- Status: Draft (investigation + proposed design)
- Scope: local/dev reference backend using CAPD (Docker)

This report investigates and proposes a detailed design to make a Cluster API `Cluster` reach `Available=True` **while the user only expresses**:

- `Cluster.spec.infrastructureRef = Kany8sCluster`
- `Cluster.spec.controlPlaneRef = Kany8sControlPlane`

and **does not** manually apply CAPD resources (`DockerCluster`, `DockerMachineTemplate`, ...) nor self-managed control plane resources (`Kany8sKubeadmControlPlane`).

The key requirement from the user:

> The goal is to create a `Cluster` and have it reach `Available=True` as-is.

Important clarification:

- CAPD (`DockerCluster`) creates a real workload cluster by running Kubernetes nodes as Docker containers, bootstrapped by kubeadm.
- Operationally that is a “self-managed” cluster.
- This proposal does **not** claim CAPD becomes a “managed control plane”.
- Instead, it makes CAPD a **backend implementation detail** behind the *Kany8s provider suite* (`Kany8sCluster` + `Kany8sControlPlane`).

---

## 1. Background: Current Entry Points and the Gap

Kany8s currently has two explicit entry points (see `docs/design.md`):

1) Managed control plane (kro mode)
   - Entry: `Kany8sControlPlane` + kro/RGD
   - Goal: control plane Ready/endpoint reflection.

2) Self-managed (kubeadm + CAPD)
   - Entry: `Cluster` + `DockerCluster` + `Kany8sKubeadmControlPlane`
   - Goal: real reachable workload cluster with `RemoteConnectionProbe=True` and `Available=True`.

Manual verification shows:

- A `Cluster` referencing `Kany8sCluster` + `Kany8sControlPlane` (kro-managed CP) does **not** create CAPD resources and therefore does **not** create a real workload cluster.
- The existing CAPD acceptance flow creates a real workload cluster, but requires the user-facing resources to be `DockerCluster` + `Kany8sKubeadmControlPlane`.

The missing capability is a facade that allows users to keep using the “Kany8s suite” expression while still provisioning a CAPD workload cluster.

---

## 2. Contract Constraint: What Makes `Cluster Available=True`

Cluster API computes `ClusterAvailable` as a summary over several conditions. In CAPI v1.12.x, `Available=True` requires (not exhaustive):

- `RemoteConnectionProbe=True` (workload API reachable using `<cluster>-kubeconfig`)
- `InfrastructureReady=True` (infra object is provisioned)
- `ControlPlaneAvailable=True` (control plane initialized)
- `WorkersAvailable=True` (no workers is acceptable; this condition can still be true)
- `TopologyReconciled=True` (only when topology/ClusterClass is used)

Therefore, **to reach `Available=True`** we must:

1) Ensure a real workload control plane exists and is reachable.
2) Ensure a valid kubeconfig secret exists:
   - Secret name: `<cluster>-kubeconfig`
   - In the same namespace as the `Cluster`
   - `type: cluster.x-k8s.io/secret` and labels as expected by CAPI.

For CAPD, this effectively means “kubeadm-based provisioning must happen somewhere”, because CAPD itself only provides infrastructure.

Implementation implication for Kany8s:

- CAPI uses the referenced infra object's `status.initialization.provisioned` as a primary signal for `InfrastructureReady`.
- CAPI uses the referenced control plane object's `status.initialization.controlPlaneInitialized` as a primary signal for `ControlPlaneAvailable`.

So the facade must ensure:

- `Kany8sCluster.status.initialization.provisioned=true` only when the infra endpoint is resolved.
- `Kany8sControlPlane.status.initialization.controlPlaneInitialized=true` only when the workload API is actually reachable (or equivalent signal).

---

## 3. Design Options Considered

### Option A: Embed CAPD-specific logic directly in Kany8s controllers

`Kany8sCluster` controller directly creates `DockerCluster`/`DockerMachineTemplate`, reads their fields, and sets `provisioned` and `controlPlaneEndpoint`.

Pros:

- No kro dependency for CAPD.
- Implementation can be straightforward.

Cons:

- Violates the core principle: Kany8s controllers should not become provider-specific.
- Adds more branching and long-term maintenance burden.

### Option B (Recommended): Keep infra provider-agnostic (kro), add a ControlPlane “kubeadm delegate” backend

Split responsibilities:

- Infra: `Kany8sCluster` remains provider-agnostic by consuming **only** a kro instance status.
  - A CAPD-specific infra RGD creates `DockerCluster` + `DockerMachineTemplate`.
  - The RGD normalizes infra readiness and exposes an endpoint.

- Control plane: `Kany8sControlPlane` gains a backend that **delegates** to the existing `Kany8sKubeadmControlPlane` controller.
  - The delegate creates/patches an internal `Kany8sKubeadmControlPlane` and mirrors readiness back to `Kany8sControlPlane`.

Pros:

- Preserves “provider-agnostic controller” for infra (all CAPD details are in RGD).
- Reuses the already working kubeadm bootstrap implementation.
- Keeps the user-facing expression stable: `Cluster` + `Kany8sCluster` + `Kany8sControlPlane`.

Cons:

- `Kany8sControlPlane` gains a second backend mode (managed kro vs kubeadm delegate).
- Requires careful ownership / status mirroring design.

### Option C: Nested CAPI (kro creates an “inner Cluster”)

Have a kro graph create a second `Cluster` that uses `DockerCluster` + `Kany8sKubeadmControlPlane`.

Cons:

- Creates two Clusters per desired workload cluster.
- Confusing ownership/UX.
- Difficult to integrate with standard `clusterctl get kubeconfig` expectations.

### Option D: Keep current split, do not unify

Do nothing.

Cons:

- Does not meet the stated requirement (“Kany8s expression only”).

---

## 4. Recommended Architecture (Option B)

### 4.1 High-level idea

Make CAPD a backend behind the Kany8s suite:

- `Kany8sCluster` (Infrastructure provider)
  - Uses kro mode with a CAPD infra RGD.
  - Becomes `Provisioned=true` when CAPD provides a control plane endpoint.
  - Exposes the endpoint in a way that a kubeadm control plane can consume.

- `Kany8sControlPlane` (ControlPlane provider)
  - New “kubeadm delegate” mode.
  - Creates/patches an internal `Kany8sKubeadmControlPlane`.
  - Mirrors `controlPlaneEndpoint`, `controlPlaneInitialized`, and `Ready` to satisfy the CAPI control plane provider contract.

The user applies only:

1) `Cluster` referencing `Kany8sCluster` and `Kany8sControlPlane`
2) `Kany8sCluster` selecting the CAPD infra RGD
3) `Kany8sControlPlane` selecting the kubeadm delegate backend

Everything else is internal.

### 4.2 Resource model (outer vs internal)

Outer (user-applied):

- `cluster.x-k8s.io/v1beta2, Kind=Cluster` (`<name>`)
- `infrastructure.cluster.x-k8s.io/v1alpha1, Kind=Kany8sCluster` (`<name>`)
- `controlplane.cluster.x-k8s.io/v1alpha1, Kind=Kany8sControlPlane` (`<name>`)

Internal (controller-created):

- `controlplane.cluster.x-k8s.io/v1alpha1, Kind=Kany8sKubeadmControlPlane` (`<name>`, owned by Cluster)
- CAPD infra resources (created by kro via RGD instance)
  - `infrastructure.cluster.x-k8s.io/v1beta2, Kind=DockerCluster` (`<name>`, includes OwnerReference to `Cluster/<name>`)
  - `infrastructure.cluster.x-k8s.io/v1beta2, Kind=DockerMachineTemplate` (`<name>-control-plane`)
- Standard CAPI objects (created by `Kany8sKubeadmControlPlane` controller)
  - `cluster.x-k8s.io/v1beta2, Kind=Machine` (control plane)
  - `bootstrap.cluster.x-k8s.io/v1beta2, Kind=KubeadmConfig`
  - kubeconfig + cert secrets

### 4.3 Naming conventions (to avoid “generic outputs”)

To avoid “infra outputs passing” for CAPD, the facade uses naming conventions:

- `DockerCluster` name = `<cluster>`
- `DockerMachineTemplate` name = `<cluster>-control-plane`
- `Kany8sKubeadmControlPlane` name = `<cluster>`

`Kany8sControlPlane` can deterministically reference the machine template without reading any provider-specific output.

### 4.4 User-facing YAML (proposed)

The following is the intended UX: the user applies only these three objects.

`Cluster` (references only Kany8s types):

```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: demo-capd
  namespace: default
spec:
  infrastructureRef:
    apiGroup: infrastructure.cluster.x-k8s.io
    kind: Kany8sCluster
    name: demo-capd
  controlPlaneRef:
    apiGroup: controlplane.cluster.x-k8s.io
    kind: Kany8sControlPlane
    name: demo-capd
```

`Kany8sCluster` (selects CAPD infra RGD; optional kroSpec for CAPD tuning):

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: Kany8sCluster
metadata:
  name: demo-capd
  namespace: default
spec:
  resourceGraphDefinitionRef:
    name: capd-infra.kro.run
  kroSpec:
    # Optional provider-specific inputs for the CAPD infra RGD.
    # Example: override node image used by DockerMachineTemplate.
    nodeImage: kindest/node:v1.34.0
```

`Kany8sControlPlane` (selects kubeadm delegate backend):

```yaml
apiVersion: controlplane.cluster.x-k8s.io/v1alpha1
kind: Kany8sControlPlane
metadata:
  name: demo-capd
  namespace: default
spec:
  version: v1.34.0
  kubeadm:
    replicas: 1
    # Optional: allow advanced users to pass through kubeadm config.
    # kubeadmConfigSpec:
    #   clusterConfiguration:
    #     apiServer:
    #       extraArgs:
    #         authorization-mode: Node,RBAC
```

Non-goal for this proposal: the user should not need to apply any of the following explicitly:

- `DockerCluster`, `DockerMachineTemplate`
- `Kany8sKubeadmControlPlane`
- `Machine`, `KubeadmConfig`, kubeconfig/cert secrets

---

## 5. Detailed Reconciliation Flow

This section describes the critical data/condition flow to reach `Cluster Available=True`.

### 5.1 Infra path (`Kany8sCluster` -> CAPD)

1) User creates `Kany8sCluster/<cluster>` with kro mode enabled.
2) `Kany8sClusterReconciler` resolves RGD-generated GVK and creates kro instance 1:1 (existing behavior), injecting cluster identity into the instance spec:
   - `spec.clusterName=<cluster>`
   - `spec.clusterNamespace=<namespace>`
   - `spec.clusterUID=<owner Cluster UID>` (new; required to set valid OwnerReferences)
3) CAPD infra RGD instance creates:
   - `DockerCluster/<cluster>` (MUST include an OwnerReference to `Cluster/<cluster>`; otherwise CAPD will not reconcile)
   - `DockerMachineTemplate/<cluster>-control-plane`

Important note:

- In this facade mode, `DockerCluster` is not referenced by `Cluster.spec.infrastructureRef`, so the CAPI Cluster controller will not add owner/label metadata to it. The infra RGD must set the Cluster OwnerReference/labels itself.
- This "Cluster identity injection" pattern is not CAPD-specific; many infrastructure providers use OwnerReferences and/or `cluster.x-k8s.io/cluster-name` to resolve the owner Cluster.
4) CAPD controller eventually sets `DockerCluster.spec.controlPlaneEndpoint.{host,port}`.
5) The infra RGD instance projects those fields into normalized instance status:
   - `status.ready=true` (only when endpoint exists)
   - `status.endpoint="https://<host>:<port>"` (or equivalent)
6) `Kany8sClusterReconciler` consumes instance status:
   - Sets `Kany8sCluster.status.initialization.provisioned = status.ready`
   - Sets `Ready` condition accordingly
   - Parses `status.endpoint` and sets `Kany8sCluster.spec.controlPlaneEndpoint` (new)

Key outcome:

- `Cluster.spec.infrastructureRef = Kany8sCluster` now points to an infra object that exposes `spec.controlPlaneEndpoint`.

### 5.2 Control plane path (`Kany8sControlPlane` -> internal `Kany8sKubeadmControlPlane`)

1) User creates `Kany8sControlPlane/<cluster>` in kubeadm delegate mode.
2) `Kany8sControlPlaneReconciler` resolves owner Cluster (via OwnerReferences set by the CAPI Cluster controller).
3) It creates/patches internal `Kany8sKubeadmControlPlane/<cluster>`:
   - OwnerReference: `Cluster/<cluster>` (required)
   - `spec.version`: from `Kany8sControlPlane.spec.version`
   - `spec.replicas`: default 1 (configurable)
   - `spec.machineTemplate.infrastructureRef`:
     - `apiGroup: infrastructure.cluster.x-k8s.io`
     - `kind: DockerMachineTemplate`
     - `name: <cluster>-control-plane`
4) `Kany8sKubeadmControlPlaneReconciler` runs (already implemented):
   - Reads infra endpoint from `Cluster.spec.infrastructureRef` (this is `Kany8sCluster.spec.controlPlaneEndpoint`)
   - Writes its own `spec.controlPlaneEndpoint`
   - Generates certificates + `<cluster>-kubeconfig`
   - Creates initial `KubeadmConfig` + `Machine`
5) CAPD provisions containers; workload API becomes reachable.
6) CAPI Cluster controller sets `Cluster.RemoteConnectionProbe=True`.
7) `Kany8sControlPlaneReconciler` mirrors readiness back:
   - Sets `Kany8sControlPlane.spec.controlPlaneEndpoint` (from internal kubeadm CP or from infra)
   - Sets `Kany8sControlPlane.status.initialization.controlPlaneInitialized` when:
     - either internal kubeadm CP reports initialized, or
     - owner `Cluster.RemoteConnectionProbe=True` (recommended signal)
   - Sets `Ready` condition when initialized.

Key outcome:

- `Cluster.ControlPlaneAvailable=True` becomes true because the referenced control plane object (`Kany8sControlPlane`) reports initialized.

### 5.3 Cluster availability

At this point, CAPI can compute:

- InfrastructureReady from `Kany8sCluster.status.initialization.provisioned`
- ControlPlaneAvailable from `Kany8sControlPlane.status.initialization.controlPlaneInitialized`
- RemoteConnectionProbe from kubeconfig secret + endpoint reachability

Thus `Cluster Available=True` is reachable.

---

## 6. Proposed API Changes

### 6.1 `Kany8sCluster` (Infrastructure)

Add an endpoint field to satisfy the expectations of kubeadm-based control planes.

Proposed field:

- `Kany8sCluster.spec.controlPlaneEndpoint: APIEndpoint` (optional; set by controller)

Concrete shape (Go, draft):

```go
// api/infrastructure/v1alpha1/kany8scluster_types.go
//
// NOTE: Many infrastructure providers use spec.controlPlaneEndpoint as the source-of-truth.
// This is consumed by kubeadm-based control plane providers.
// +optional
ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint,omitempty"`
```

Practical note:

- In Go's `encoding/json`, `omitempty` does not omit non-pointer structs.
- If we want to omit empty endpoints in serialized YAML/JSON, prefer `*clusterv1.APIEndpoint`.
- This is not required for correctness; it is UX/readability.

Rationale:

- Many infra providers expose `spec.controlPlaneEndpoint` as the source-of-truth for the control plane endpoint.
- `Kany8sKubeadmControlPlaneReconciler` already reads `spec.controlPlaneEndpoint` from the infra object referenced by `Cluster.spec.infrastructureRef`.
- For CAPD facade, `Cluster.spec.infrastructureRef` remains `Kany8sCluster`, so `Kany8sCluster` must carry this field.

### 6.2 `Kany8sControlPlane` (ControlPlane)

Add a way to select kubeadm delegate backend.

Two viable schema designs:

1) `spec.kubeadm` block (presence selects backend)
2) `spec.backend.type` enum (`kro` default, `kubeadm` alternative)

Recommended (simpler, minimal disruption):

- Add `spec.kubeadm` optional struct.
- Make `spec.resourceGraphDefinitionRef` optional.
- Controller behavior:
  - If `spec.kubeadm != nil`: kubeadm delegate mode.
  - Else: current kro mode; require `resourceGraphDefinitionRef`.

Validation:

- Add an optional webhook (or CEL validations if feasible) to enforce:
  - exactly one backend is selected.
  - `resourceGraphDefinitionRef` required when `spec.kubeadm` is nil.

Concrete shape (Go, draft):

```go
// api/v1alpha1/kany8scontrolplane_types.go

type Kany8sControlPlaneKubeadmBackend struct {
  // replicas is the desired number of control plane Machines.
  // +kubebuilder:validation:Minimum=1
  // +kubebuilder:default=1
  // +optional
  Replicas *int32 `json:"replicas,omitempty"`

  // kubeadmConfigSpec is an optional passthrough for kubeadm bootstrap.
  // +optional
  KubeadmConfigSpec *bootstrapv1.KubeadmConfigSpec `json:"kubeadmConfigSpec,omitempty"`

  // machineTemplate allows overriding the infra machine template reference.
  // If unset, default is provider-specific but deterministic; for CAPD facade:
  // - kind: DockerMachineTemplate
  // - name: <cluster>-control-plane
  // +optional
  MachineTemplate *Kany8sKubeadmControlPlaneMachineTemplate `json:"machineTemplate,omitempty"`
}

type Kany8sControlPlaneSpec struct {
  Version string `json:"version"`

  // kro backend (current)
  // +optional
  ResourceGraphDefinitionRef *ResourceGraphDefinitionReference `json:"resourceGraphDefinitionRef,omitempty"`
  // +optional
  KroSpec *apiextensionsv1.JSON `json:"kroSpec,omitempty"`

  // kubeadm delegate backend (new)
  // +optional
  Kubeadm *Kany8sControlPlaneKubeadmBackend `json:"kubeadm,omitempty"`

  // output
  // +optional
  ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint,omitempty"`
}
```

Compatibility note:

- This keeps existing fields intact but changes requiredness.
- Existing managed(kro) flows must be adjusted to ensure `resourceGraphDefinitionRef` is set.
- Controller must return a clear terminal error when neither backend is properly configured.

---

## 7. Proposed Controller Changes

### 7.1 `Kany8sClusterReconciler`

Current file: `internal/controller/infrastructure/kany8scluster_controller.go`

Additions:

- When in kro mode and instance reports `status.endpoint`, parse it and set:
  - `Kany8sCluster.spec.controlPlaneEndpoint.host/port`
- Keep existing behavior:
  - `status.initialization.provisioned` mirrors instance `status.ready`
  - `Ready` condition mirrors provisioned
  - `failureReason/failureMessage` terminal-only

Notes:

- Parsing should reuse `internal/endpoint` utilities (same as `Kany8sControlPlane`).
- Endpoint should only be written when it is valid and non-empty; avoid flipping back to empty on transient reads.

### 7.2 `Kany8sControlPlaneReconciler`

Current file: `internal/controller/kany8scontrolplane_controller.go`

Additions:

- Branch based on backend selection:
  - kro mode: unchanged.
  - kubeadm delegate mode: new reconcile path.

Delegate mode details:

1) Resolve owner `Cluster` (via OwnerReferences; same strategy as `Kany8sKubeadmControlPlane`).
2) Create/patch internal `Kany8sKubeadmControlPlane` with:
   - OwnerReference: Cluster
   - `spec.version` from `Kany8sControlPlane.spec.version`
   - `spec.replicas` default 1 (configurable)
   - `spec.machineTemplate.infrastructureRef` points to `DockerMachineTemplate/<cluster>-control-plane`
3) Mirror readiness back to `Kany8sControlPlane`:
   - `spec.controlPlaneEndpoint` from internal CP (or from infra)
   - `status.initialization.controlPlaneInitialized` based on:
     - `Cluster.RemoteConnectionProbe=True` (preferred)
   - `Ready` condition set when initialized.
4) Failure propagation:
- If internal CP has terminal failure, surface it in `Kany8sControlPlane.status.failure*`.
- If infra is not provisioned, surface waiting reason (non-terminal).

Suggested mirroring rules (draft):

- `Kany8sControlPlane.spec.controlPlaneEndpoint`:
  - Prefer internal `Kany8sKubeadmControlPlane.spec.controlPlaneEndpoint` when populated.
  - Fallback to infra endpoint (`Kany8sCluster.spec.controlPlaneEndpoint`) if needed.

- `Kany8sControlPlane.status.initialization.controlPlaneInitialized`:
  - Prefer `Cluster.RemoteConnectionProbe=True` as the signal.
  - Rationale: `Available=True` requires RemoteConnectionProbe anyway; using the same signal avoids subtle mismatches.

- `Kany8sControlPlane Ready` condition:
  - `True` when `controlPlaneInitialized=true`.
  - `False` with explicit reasons when waiting for:
    - owner Cluster
    - infra endpoint
    - remote connection probe

Pseudo-code (delegate backend):

```text
reconcile(cp):
  cluster = getOwnerCluster(cp)
  if cluster == nil:
    set Ready=False reason=WaitingForOwnerCluster
    return requeue

  ensure internal Kany8sKubeadmControlPlane exists:
    name = cluster.Name
    namespace = cluster.Namespace
    ownerRef = Cluster
    spec.version = cp.spec.version
    spec.replicas = cp.spec.kubeadm.replicas ?? 1
    spec.kubeadmConfigSpec = cp.spec.kubeadm.kubeadmConfigSpec (optional)
    spec.machineTemplate.infrastructureRef =
      cp.spec.kubeadm.machineTemplate?.infrastructureRef ??
      {apiGroup: infra.cluster.x-k8s.io, kind: DockerMachineTemplate, name: <cluster>-control-plane}

  endpoint = internalCP.spec.controlPlaneEndpoint
  if endpoint missing:
    endpoint = infraObject.spec.controlPlaneEndpoint (best-effort)
  if endpoint present:
    patch cp.spec.controlPlaneEndpoint

  if Cluster.RemoteConnectionProbe=True:
    patch cp.status.initialization.controlPlaneInitialized=true
    set Ready=True
  else:
    set Ready=False reason=WaitingForRemoteConnectionProbe
    requeue
```

Watches and event sources (draft):

- Primary: `Kany8sControlPlane`
- Secondary:
  - Owner `Cluster` (watch to reconcile when RemoteConnectionProbe flips)
  - Internal `Kany8sKubeadmControlPlane` (watch to reconcile when endpoint/conditions change)

Implementation note:

- Internal `Kany8sKubeadmControlPlane` should include both OwnerReferences:
  - `Cluster` (required for its controller)
  - `Kany8sControlPlane` (optional but useful to enable `Owns()` watch wiring)

Watches:

- Watch `Cluster` status changes (so RemoteConnectionProbe updates trigger reconcile).
- Watch internal `Kany8sKubeadmControlPlane` changes.

RBAC:

- Add permissions for `kany8skubeadmcontrolplanes` get/list/watch/create/update/patch and status.
- Likely need read access to `cluster.x-k8s.io` Clusters (get/list/watch).

---

## 8. CAPD Infra RGD Design (kro)

We need a CAPD infra RGD that produces a “normalized infra instance” for `Kany8sCluster`.

Desired properties:

- Creates `DockerCluster` and `DockerMachineTemplate`.
- Ensures CAPD can associate `DockerCluster` with the owning CAPI `Cluster` by setting:
  - `metadata.ownerReferences[]` to `Cluster/<cluster>` (requires `clusterUID` in the instance spec)
  - `metadata.labels["cluster.x-k8s.io/cluster-name"]=<cluster>` (recommended)
- Produces status fields:
  - `status.ready: boolean` true when endpoint host+port exist.
  - `status.endpoint: string` formatted as `https://host:port` (or `host:port`).
  - `status.reason/message` optional.

Implementation notes (kro pitfalls):

- `readyWhen` cannot reference other resources; do not rely on it.
- Status expressions must tolerate missing fields while CAPD is still converging.
    - Use safe navigation and defaulting (`.?`, `orValue(...)`, etc.) per `docs/reference/rgd-guidelines.md`.

This RGD is an implementation detail; users select it via `Kany8sCluster.spec.resourceGraphDefinitionRef`.

Example skeleton (illustrative; not final):

```yaml
apiVersion: kro.run/v1alpha1
kind: ResourceGraphDefinition
metadata:
  name: capd-infra.kro.run
spec:
  schema:
    apiVersion: v1alpha1
    kind: CapdInfrastructure
    spec:
      clusterName: string | required=true description="CAPI Cluster name"
      clusterNamespace: string | required=true description="CAPI Cluster namespace"
      clusterUID: string | required=true description="CAPI Cluster UID (for OwnerReferences)"
      nodeImage: string | default="kindest/node:v1.34.0" description="DockerMachineTemplate customImage"
    status:
      endpoint: '${
        ((dockerCluster.?spec.?controlPlaneEndpoint.?host.orValue("") != "")
        && (int(dockerCluster.?spec.?controlPlaneEndpoint.?port.orValue(0)) > 0))
        ? ("https://" + dockerCluster.?spec.?controlPlaneEndpoint.?host.orValue("")
          + ":" + string(int(dockerCluster.?spec.?controlPlaneEndpoint.?port.orValue(0))))
        : ""
      }'
      ready: '${int(
        (((dockerCluster.?spec.?controlPlaneEndpoint.?host.orValue("") != "")
        && (int(dockerCluster.?spec.?controlPlaneEndpoint.?port.orValue(0)) > 0))
        ? 1 : 0)
      ) == 1}'
      reason: '${
        ((dockerCluster.?spec.?controlPlaneEndpoint.?host.orValue("") != "")
        && (int(dockerCluster.?spec.?controlPlaneEndpoint.?port.orValue(0)) > 0))
        ? "Ready" : "Provisioning"
      }'
      message: '${
        ((dockerCluster.?spec.?controlPlaneEndpoint.?host.orValue("") != "")
        && (int(dockerCluster.?spec.?controlPlaneEndpoint.?port.orValue(0)) > 0))
        ? "endpoint is ready" : "waiting for CAPD to set controlPlaneEndpoint"
      }'
  resources:
    - id: dockerCluster
      template:
        apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
        kind: DockerCluster
        metadata:
          name: ${schema.spec.clusterName}
          namespace: ${schema.spec.clusterNamespace}
          labels:
            kany8s.io/cluster-name: ${schema.spec.clusterName}
            cluster.x-k8s.io/cluster-name: ${schema.spec.clusterName}
          ownerReferences:
            - apiVersion: cluster.x-k8s.io/v1beta2
              kind: Cluster
              name: ${schema.spec.clusterName}
              uid: ${schema.spec.clusterUID}
        spec: {}
    - id: dockerMachineTemplate
      template:
        apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
        kind: DockerMachineTemplate
        metadata:
          name: ${schema.spec.clusterName}-control-plane
          namespace: ${schema.spec.clusterNamespace}
          labels:
            kany8s.io/cluster-name: ${schema.spec.clusterName}
            cluster.x-k8s.io/cluster-name: ${schema.spec.clusterName}
        spec:
          template:
            spec:
              customImage: ${schema.spec.nodeImage}
```

Notes:

- This RGD intentionally does not attempt to drive provisioning directly; CAPD controllers do that.
- The OwnerReference + `cluster.x-k8s.io/cluster-name` label are required so CAPD can reconcile resources created "behind" the facade.
- `status.endpoint` intentionally tolerates missing host/port while CAPD is still converging.
- `status.ready` should be treated as “infra endpoint is ready” (not “workload is reachable”).

---

## 9. Acceptance Test Plan (New)

Add a new acceptance path that proves the facade end-to-end:

Goal:

- User applies only `Cluster` + `Kany8sCluster` + `Kany8sControlPlane`.
- The `Cluster` reaches:
  - `RemoteConnectionProbe=True`
  - `Available=True`
- Workload kubeconfig works (`clusterctl get kubeconfig` + `kubectl get nodes`).
- CAPD containers exist on host Docker.

Test environment requirements:

- Kind management cluster with `/var/run/docker.sock` mounted into node (CAPD requirement).
- Providers installed:
  - CAPI core
  - CABPK (`--bootstrap kubeadm`)
  - CAPD (`--infrastructure docker`)
  - Kany8s (`--control-plane kany8s`)
- kro installed + CAPD infra RGD applied (if using kro for infra).

This can be implemented as a new script under `hack/` and a wrapper under `test/acceptance_test/`, similar to existing patterns.

Suggested acceptance script outline (draft):

1) Create a kind management cluster with docker.sock mounted
   - same approach as `hack/acceptance-test-capd-kubeadm.sh`
2) Build + load Kany8s controller image
3) Install providers via clusterctl:
   - `clusterctl init --infrastructure docker --bootstrap kubeadm --control-plane kany8s`
4) Install kro (if infra uses kro) and apply CAPD infra RGD:
   - `kubectl apply -f capd-infra.rgd.yaml`
   - wait `ResourceGraphAccepted`
5) Apply user manifests (`Cluster` + `Kany8sCluster` + `Kany8sControlPlane`)
6) Wait for conditions:
   - `kubectl get kany8scluster <name> -o yaml` shows `provisioned=true`
   - `kubectl get kany8scontrolplane <name> -o yaml` shows initialized/Ready
   - `kubectl get cluster <name> -o yaml` shows `RemoteConnectionProbe=True` and `Available=True`
7) Fetch workload kubeconfig and connect:
   - `clusterctl get kubeconfig -n <ns> <cluster> > workload.kubeconfig`
   - `kubectl --kubeconfig workload.kubeconfig get nodes`
8) Verify CAPD created workload containers:
   - `docker ps --format '{{.Names}}' | rg '^<cluster>-'`
9) Cleanup:
   - delete kind cluster
   - delete workload containers (CAPD leaves them on host)

---

## 10. Risks and Mitigations

1) CAPD requires Docker socket access
   - Only feasible for local/dev clusters.
   - Acceptance must mount docker.sock into kind node.

2) Dual control plane objects may confuse users
   - Clearly label internal objects with annotations like `kany8s.io/internal=true`.
   - Document that `Kany8sKubeadmControlPlane` is internal in this facade mode.

3) Status/condition semantics diverge between managed and delegate modes
   - Make backend selection explicit in `Kany8sControlPlane.spec`.
   - Define readiness semantics per backend.

4) Validation complexity
   - Start with controller-side validation and clear errors.
   - Add webhook later if needed.

---

## 11. Open Questions

1) How configurable should the CAPD facade be?
   - Node image (`DockerMachineTemplate.spec.template.spec.customImage`)
   - kubeadm config customization
   - replicas > 1

2) Should `Kany8sCluster` set endpoint in `spec` vs `status`?
   - kubeadm CP expects `spec.controlPlaneEndpoint`, so `spec` is required unless we also change the kubeadm controller.

3) Should infra be required to use kro for CAPD?
   - This report recommends kro to keep provider-specifics out of controllers.
   - A direct CAPD backend could be introduced as an alternative MVP if kro dependency is undesirable.

---

## 12. Proposed Next Steps

1) Add `Kany8sCluster.spec.controlPlaneEndpoint` and implement endpoint propagation from infra readiness.
2) Add `Kany8sControlPlane` kubeadm delegate backend (API + controller + RBAC + tests).
3) Add CAPD infra RGD with Cluster ownership metadata (OwnerReference + `cluster.x-k8s.io/cluster-name`) and extend instance spec injection to include `clusterUID`.
4) Add new acceptance test proving the facade end-to-end (`Cluster Available=True`).
