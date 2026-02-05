# Parent RGD Includes Infra (Approach A)

- Date: 2026-02-02
- Context: Approach A in `docs/PRD-details.md`

## Goal

- Keep infra -> control plane value passing inside a single kro graph (parent RGD).
- Avoid passing infra outputs between Kany8s CRDs (no "generic outputs" in Kany8s core).
- Keep Kany8s provider-agnostic: Kany8s reads only normalized kro instance status.

## Pattern

- `Kany8sControlPlane` references a parent RGD instance.
- The parent RGD composes:
  - `network`: infra RGD instance (example kind: `AWSNetwork`)
  - `controlPlane`: control plane RGD instance (example kind: `EKSControlPlane`)
- The parent RGD exposes the normalized status contract for Kany8s:
  - `status.ready: boolean`
  - `status.endpoint: string`
  by projecting from `controlPlane.status`.

## Data Flow

1. `Kany8sControlPlane` reconciler creates/updates the parent kro instance (1:1) and injects `.spec.version`.
2. Parent instance creates `network` (infra).
3. Parent passes infra outputs via in-graph references (`network.status.*` -> `controlPlane.spec.*`).
4. Parent projects `controlPlane.status.ready/endpoint` into its own `status.ready/endpoint`.
5. Kany8s consumes parent `status.ready/endpoint` only (per `docs/rgd-contract.md`).

## Sample: Parent RGD (infra + control plane)

```yaml
apiVersion: kro.run/v1alpha1
kind: ResourceGraphDefinition
metadata:
  name: eks-control-plane-with-infra.kro.run
spec:
  schema:
    apiVersion: v1alpha1
    kind: EKSControlPlaneWithInfra
    spec:
      region: string | required=true description="AWS region"
      version: string | required=true description="Kubernetes version (Kany8s injects this)"
      network:
        vpcCIDR: string | default="10.0.0.0/16"
        privateSubnetCIDRs: '[]string | default=["10.0.0.0/20","10.0.16.0/20"]'

    # Normalized status contract for Kany8sControlPlane.
    # Note: kro v0.7.x may drop boolean fields; use int/ternary to keep the field present.
    status:
      endpoint: ${controlPlane.?status.?endpoint.orValue("")}
      ready: '${int((controlPlane.?status.?ready.orValue(false)) ? 1 : 0) == 1}'

  resources:
    # infra: VPC/Subnets/SG (provider-specific implementation)
    - id: network
      template:
        apiVersion: kro.run/v1alpha1
        kind: AWSNetwork
        metadata:
          name: ${schema.metadata.name}-network
        spec:
          region: ${schema.spec.region}
          vpcCIDR: ${schema.spec.network.vpcCIDR}
          privateSubnetCIDRs: ${schema.spec.network.privateSubnetCIDRs}
      # Use `status.state` to avoid missing-field pitfalls.
      readyWhen:
        - ${network.status.state == "ACTIVE"}

    # control plane: consumes infra outputs inside the same graph
    - id: controlPlane
      template:
        apiVersion: kro.run/v1alpha1
        kind: EKSControlPlane
        metadata:
          name: ${schema.metadata.name}
        spec:
          version: ${schema.spec.version}
          vpc:
            subnetIDs: ${network.status.subnetIDs}
            securityGroupIDs: ${network.status.securityGroupIDs}
```

## AWSNetwork minimal status contract (example)

The infra RGD (here: `AWSNetwork`) is provider-specific (ACK EC2 / Crossplane / etc.).
To support the parent pattern, it should expose typed outputs via its instance status:

- `status.subnetIDs: []string` (required)
- `status.securityGroupIDs: []string` (required)
- `status.ready: boolean` (recommended)
- `status.reason: string` (optional)
- `status.message: string` (optional)

## Notes / pitfalls

- For kro v0.7.x: boolean status fields may not materialize. Prefer the `int/ternary` pattern for `status.ready`.
- Avoid making a resource optional (`includeWhen=false`) if you project its status into required parent status fields; see `docs/rgd-guidelines.md`.
- Prefer passing existing VPC inputs directly (no infra outputs) when possible; use this pattern only when infra outputs are unavoidable.

## Example: Kany8sControlPlane usage

```yaml
apiVersion: controlplane.cluster.x-k8s.io/v1alpha1
kind: Kany8sControlPlane
metadata:
  name: demo-eks
  namespace: default
spec:
  version: "1.34"
  resourceGraphDefinitionRef:
    name: eks-control-plane-with-infra.kro.run
  kroSpec:
    region: ap-northeast-1
    network:
      vpcCIDR: 10.0.0.0/16
      privateSubnetCIDRs:
        - 10.0.0.0/20
        - 10.0.16.0/20
```
