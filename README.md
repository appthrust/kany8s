# Kany8s

> Any k8s, powered by kro.

**Kany8s** is a (work-in-progress) Cluster API provider suite — `Kany8sCluster` (Infrastructure) + `Kany8sControlPlane` (ControlPlane) — that uses **kro** (ResourceGraphDefinition / RGD) as a "concretization engine" to create managed Kubernetes control planes (and their prerequisites) on *any* cloud/provider.

The goal is simple: **if you can express it as a kro RGD, Kany8s can drive it via Cluster API**.

- Name: `Kany8s` = "k(ro)" + "any" + "k8s" (and it’s pronounceable)
- Repo status: design-first / prototype

## Concept

Kany8s separates responsibilities clearly:

- **Cluster API-facing CRDs**
  - `Kany8sCluster`: Infrastructure provider (referenced by `Cluster.spec.infrastructureRef`)
  - `Kany8sControlPlane`: ControlPlane provider (sets endpoint/initialized/conditions per the CAPI contract)
- **kro RGD (provider-specific)**: materializes real resources (EKS/ACK today, AKS/GKE tomorrow)
  - Hides provider-specific status shapes
  - Exposes a small, normalized status contract that Kany8s consumes

This keeps the controller provider-agnostic: **no “if EKS then … else if GKE then …” branches**.

## Architecture (High Level)

1. You create a Cluster API `Cluster` that references `Kany8sCluster` + `Kany8sControlPlane`.
2. `Kany8sControlPlane` references a kro `ResourceGraphDefinition` via `spec.resourceGraphDefinitionRef`.
3. Kany8s resolves the RGD’s generated GVK and creates exactly one **kro instance** (1:1).
4. Kany8s watches **only** the kro instance `status`.
5. When the kro instance reports ready + endpoint, Kany8s writes `Kany8sControlPlane.spec.controlPlaneEndpoint` and sets `status.initialization.controlPlaneInitialized` (Cluster controller then mirrors the endpoint into `Cluster.spec.controlPlaneEndpoint` per the CAPI contract).

A Cluster API `Cluster` will look like this:

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
metadata:
  name: demo-cluster
spec:
  infrastructureRef:
    apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
    kind: Kany8sCluster
    name: demo-cluster
  controlPlaneRef:
    apiVersion: controlplane.cluster.x-k8s.io/v1alpha1
    kind: Kany8sControlPlane
    name: demo-cluster
```

## ClusterTopology / ClusterClass (Planned)

Kany8s is designed to be consumed via Cluster API **ClusterTopology** (**ClusterClass**).

- `Kany8sControlPlaneTemplate` selects the provider implementation via `resourceGraphDefinitionRef` and carries default `kroSpec`.
- `Kany8sClusterTemplate` provides the InfrastructureRef required by Cluster API (minimal first; may later materialize shared prerequisites).
- `Cluster.spec.topology.version` is the single source of truth for `Kany8sControlPlane.spec.version` (and is injected into the kro instance `spec.version`).

A typical topology setup will look like:

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: ClusterClass
metadata:
  name: kany8s-eks
spec:
  infrastructure:
    ref:
      apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
      kind: Kany8sClusterTemplate
      name: kany8s-aws
  controlPlane:
    ref:
      apiVersion: controlplane.cluster.x-k8s.io/v1alpha1
      kind: Kany8sControlPlaneTemplate
      name: kany8s-eks
  # variables + patches map into `.spec.kroSpec` (details TBD)
```

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
metadata:
  name: demo-cluster
spec:
  topology:
    class: kany8s-eks
    version: "1.34"
    variables:
      - name: region
        value: ap-northeast-1
      - name: vpc.subnetIDs
        value: ["subnet-xxxx", "subnet-yyyy"]
      - name: vpc.securityGroupIDs
        value: ["sg-zzzz"]
```

## Contract: kro instance status (Normalized)

Kany8s expects the referenced RGD instance to expose these fields:

- `status.ready: boolean`
  - Meaning: "ControlPlane ready" (at minimum, the API endpoint is known)
- `status.endpoint: string`
  - Format: `https://host[:port]` or `host[:port]`
  - If port is omitted, Kany8s treats it as `443`
- (optional) `status.reason: string`
- (optional) `status.message: string`

Note: kro adds reserved fields like `status.conditions` and `status.state` automatically, so Kany8s uses the dedicated names above (`ready/endpoint/reason/message`).

## Example (Planned API)

### Kany8s ControlPlane

```yaml
apiVersion: controlplane.cluster.x-k8s.io/v1alpha1
kind: Kany8sControlPlane
metadata:
  name: demo-cluster
  namespace: default
spec:
  version: "1.34"
  # `controlPlaneEndpoint` is set by Kany8s (CAPI contract)
  # controlPlaneEndpoint:
  #   host: example.eks.amazonaws.com
  #   port: 443
  resourceGraphDefinitionRef:
    name: eks-control-plane
  kroSpec:
    region: ap-northeast-1
    vpc:
      subnetIDs:
        - subnet-xxxx
        - subnet-yyyy
      securityGroupIDs:
        - sg-zzzz
```

### Generated kro instance (GVK is resolved from the RGD)

```yaml
apiVersion: kro.run/v1alpha1
kind: EKSControlPlane
metadata:
  name: demo-cluster
  namespace: default
spec:
  version: "1.34" # injected/overwritten by Kany8s
  region: ap-northeast-1
  vpc:
    subnetIDs:
      - subnet-xxxx
      - subnet-yyyy
    securityGroupIDs:
      - sg-zzzz
```

### Normalizing status in the RGD (example idea)

```yaml
schema:
  status:
    ready: ${cluster.status.status == "ACTIVE" && cluster.status.endpoint != ""}
    endpoint: ${cluster.status.endpoint}
```

## Scope (MVP)

- MVP focuses on **ControlPlane provider** responsibilities (`Kany8sControlPlane`: endpoint/initialized/conditions)
- Implements `spec.controlPlaneEndpoint` + `status.initialization.controlPlaneInitialized` per the CAPI contract
- `Kany8sCluster` (Infrastructure provider) is planned/TBD
- Keeps provider-specific logic inside RGD(s)
- Does **not** adopt CAPT’s Terraform-style "Template → Apply" pattern as a core concept
- Does **not** write Terraform-like outputs to Secrets for endpoint/initialized (for now)
- Kubeconfig secret management (`<cluster>-kubeconfig`) is required by the CAPI contract (planned)

## Documents

- `design.md`: architecture and controller ↔ kro contract
- `idea.md`: ACK CR examples and RGD modularization ideas
- `capt/`: reference implementation (Cluster API Provider Terraform) used for comparison

## Roadmap (Sketch)

- Implement `Kany8sControlPlane` CRD + controller
- Implement `Kany8sCluster` CRD + controller (optional/minimal first)
- Provide a working AWS/EKS RGD (`eks-control-plane`) as a reference
- Add clusterctl/helm packaging
- Add ClusterTopology/ClusterClass examples (templates + patches)
- Extend RGD catalog for other providers (AKS/GKE/etc.)

## License

TBD
