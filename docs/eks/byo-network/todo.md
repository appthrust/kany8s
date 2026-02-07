# TODO: Implement EKS BYO Network (ClusterClass/Topology)

- [x] Re-read `docs/eks/byo-network/design.md` and freeze the concrete API names used in manifests (RGD names, schema.kind, ClusterClass name, variable names, patch paths).
- [x] Add infra BYO input-gate RGD manifest at `docs/eks/byo-network/manifests/aws-byo-network-rgd.yaml` implementing `aws-byo-network.kro.run` / kind `AWSBYONetwork` (ConfigMap only; no AWS resources) and exposing `status.ready` (bool) + `status.reason` + `status.message` per `docs/reference/rgd-contract.md`.
- [x] Validate the infra RGD against kro pitfalls in `docs/reference/rgd-guidelines.md` (no `schema.*` in `spec.schema.status`, avoid constant-only status fields, use boolean materialization workaround, use `.?`/`orValue(...)` where missing fields are possible).
- [x] Add control plane BYO RGD manifest at `docs/eks/byo-network/manifests/eks-control-plane-byo-rgd.yaml` implementing `eks-control-plane-byo.kro.run` / kind `EKSControlPlaneBYO` (ACK IAM Role + ACK EKS Cluster; no VPC/Subnet creation) and exposing normalized `status.ready`/`status.endpoint` (+ reason/message recommended).
- [x] Verify the ACK EKS Cluster spec fields used in the BYO RGD match the versions installed by `docs/eks/README.md` (especially `resourcesVPCConfig.subnetIDs` vs `subnetRefs`, `roleRef` vs `roleARN`, and region annotations).
- [x] Create an end-to-end ClusterClass manifest at `docs/eks/byo-network/manifests/clusterclass-eks-byo.yaml` containing `Kany8sClusterTemplate`, `Kany8sControlPlaneTemplate`, and `ClusterClass` for this design.
- [x] Ensure the templates in `docs/eks/byo-network/manifests/clusterclass-eks-byo.yaml` make JSON patches safe by pre-creating intermediate objects (e.g., set `kroSpec: { vpc: {} }` so patches to `/spec/template/spec/kroSpec/vpc/subnetIDs` don’t fail).
- [x] Define ClusterClass variables in `docs/eks/byo-network/manifests/clusterclass-eks-byo.yaml` as the single source of truth: `region`, `eks-version`, `vpc-subnet-ids` (minItems=2), `vpc-security-group-ids` (allow empty list), `eks-public-access-cidrs` (minItems=1; explicit).
- [x] Implement ClusterClass patches in `docs/eks/byo-network/manifests/clusterclass-eks-byo.yaml` to flow variables into both sides (RGD refs are set on the templates).
- [x] Ensure infra patches add `kroSpec.vpc.subnetIDs` / `kroSpec.vpc.securityGroupIDs` from variables.
- [x] Ensure control plane patches add `kroSpec.region`, `kroSpec.vpc.subnetIDs`, `kroSpec.vpc.securityGroupIDs`, `kroSpec.publicAccessCIDRs` from variables.
- [x] Add a BYO Cluster instance template at `docs/eks/byo-network/manifests/cluster.yaml.tpl` (Topology-only `Cluster`), with placeholders for the variables used in `docs/eks/byo-network/manifests/clusterclass-eks-byo.yaml`.
- [x] Update `docs/eks/byo-network/design.md` to reference the concrete manifest paths added under `docs/eks/byo-network/manifests/` (instead of only inline snippets).
- [x] Update `docs/eks/README.md` with a BYO network section that points to `docs/eks/byo-network/design.md` and uses the new manifests (`docs/eks/byo-network/manifests/*`).
- [x] Update `docs/eks/values.md` to include BYO-specific required values (subnet IDs, optional SG IDs, explicit `PUBLIC_ACCESS_CIDR`) and an example mapping to Topology variables.
- [x] Update `docs/eks/cleanup.md` to clarify BYO semantics: deleting the Cluster removes EKS/IAM (ACK-managed) but never touches existing VPC/Subnet (not managed by the BYO infra RGD).
- [ ] (Manual) Create a kind management cluster and install CAPI core + cert-manager + kro + ACK (follow `docs/eks/README.md`).
- [ ] (Manual) `kubectl apply -f docs/eks/byo-network/manifests/aws-byo-network-rgd.yaml` and confirm `ResourceGraphAccepted=True`.
- [ ] (Manual) `kubectl apply -f docs/eks/byo-network/manifests/eks-control-plane-byo-rgd.yaml` and confirm `ResourceGraphAccepted=True`.
- [ ] (Manual) `kubectl -n "$NAMESPACE" apply -f docs/eks/byo-network/manifests/clusterclass-eks-byo.yaml` and confirm ClusterClass is accepted.
- [ ] (Manual) Render/apply a BYO Topology Cluster from `docs/eks/byo-network/manifests/cluster.yaml.tpl` and confirm topology renders `Kany8sCluster` + `Kany8sControlPlane` with the expected RGDs and patched `kroSpec`.
- [ ] (Manual) Confirm readiness behavior: `Kany8sCluster.status.initialization.provisioned` gates on inputs (>=2 subnets), and `Kany8sControlPlane` Ready gates on EKS ACTIVE + endpoint.
- [ ] (Manual) Delete the BYO Cluster and confirm ACK deletes EKS Cluster + IAM Role; confirm existing VPC/Subnet remain unchanged.
- [x] Update `docs/issues/eks-byo-network-infrastructurecluster.md` to link to `docs/eks/byo-network/design.md`, list the new manifests, and mark the issue Closed once the manual validation succeeds.

## Review notes (2026-02-07)

- [ ] Review (OK): `docs/eks/byo-network/manifests/aws-byo-network-rgd.yaml` is ConfigMap-only input-gate; deletion safety for existing VPC/Subnet is preserved; boolean materialization workaround is applied.
- [ ] Review (OK): `docs/eks/byo-network/manifests/clusterclass-eks-byo.yaml` pre-creates `kroSpec.vpc` so JSON patches to `/spec/template/spec/kroSpec/vpc/*` are safe.
- [x] Review (Fix): BYO uses `schema.kind: EKSControlPlaneBYO`, so kro instance resource name differs from `ekscontrolplanes.kro.run`; update `docs/eks/README.md` and `docs/eks/cleanup.md` to avoid hardcoding the smoke-only resource name and instead reference the correct resource (or instruct discovery via `kubectl api-resources --api-group=kro.run`).
- [x] Review (Fix): Align namespace assumptions: `docs/eks/byo-network/manifests/clusterclass-eks-byo.yaml` omits `metadata.namespace`; apply it with `kubectl -n "$NAMESPACE" ...` and set `__NAMESPACE__` in `docs/eks/byo-network/manifests/cluster.yaml.tpl` to the same namespace.
- [x] Review (Fix): Confirm ClusterClass/Topology prerequisites for the CAPI version used in `docs/eks/README.md` (feature gate / controller enablement) and document any required setup.
- [x] Review (Optional): Add BYO-specific "progress view" commands (including the BYO kro instance resource name) or clearly label the existing smoke-only troubleshooting commands in `docs/eks/README.md`.
