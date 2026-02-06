# ADR 0004: Kubeconfig Secret Strategy

- Status: Accepted
- Date: 2026-02-05

## Context

Cluster API requires a CAPI-compatible kubeconfig Secret:

- Secret name: `<cluster>-kubeconfig`
- Secret type: `cluster.x-k8s.io/secret`
- Secret label: `cluster.x-k8s.io/cluster-name: <cluster>`
- Kubeconfig is stored at `data.value`

For managed control planes driven by kro, the kubeconfig may be produced by a provider-specific controller (ACK, etc.) or by an RGD itself.

## Decision

We explicitly document two options, and we adopt Option B for the default Kany8s controller behavior.

### Option A: RGD creates kubeconfig Secret

RGD creates the CAPI-compatible `<cluster>-kubeconfig` Secret directly.

Requirements (must satisfy CAPI expectations):

- `type: cluster.x-k8s.io/secret`
- `cluster.x-k8s.io/cluster-name:` label
- `stringData:` / `data.value` contains kubeconfig
- Secret is in the same namespace as the `Cluster` / `Kany8sControlPlane`

Example (shape only):

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: demo-cluster-kubeconfig
  namespace: default
  labels:
    cluster.x-k8s.io/cluster-name: demo-cluster
type: cluster.x-k8s.io/secret
stringData:
  value: |
    apiVersion: v1
    kind: Config
```

### Option B: Kany8s creates kubeconfig Secret

RGD (or underlying provider controllers) creates a provider-specific source Secret and exposes a reference via:

- `status.kubeconfigSecretRef` (on the kro instance)

Kany8s reads the source Secret's `data.value` and writes/updates the CAPI-compatible `<cluster>-kubeconfig` Secret.

Namespace / security guidance:

- The source Secret SHOULD be in the same namespace as the `Cluster` / `Kany8sControlPlane` (and the kro instance).
- If a reference points to a different namespace (i.e., `kubeconfigSecretRef.namespace` differs), the installation MUST explicitly opt in by extending RBAC. This is discouraged by default because it creates a cross-namespace Secret read path.

Key properties:

- The source Secret can be provider-specific (metadata contract is minimal).
- Kany8s owns the CAPI kubeconfig Secret shape and keeps it consistent.

## Consequences

- RGD authors can keep kubeconfig production provider-specific, while still enabling CAPI compatibility.
- Kany8s can validate kubeconfig content before surfacing it to CAPI.
