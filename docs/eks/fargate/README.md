# EKS: Fargate bootstrap + Karpenter (NodeGroupなし)

このディレクトリは `docs/eks/byo-network/` の BYO EKS ControlPlane に対して、

- `kube-system/coredns` と `karpenter` を EKS Fargate で起動（ノード0でも起動可能にする）
- Karpenter で EC2 worker を自動作成

するための設計/実装メモです。

## 読む順番

- プラン: `docs/eks/fargate/plan.md`
- 設計: `docs/eks/fargate/design.md`
- 実装 TODO: `docs/eks/fargate/todo.md`
- 実行ログ: `docs/eks/fargate/wip.md`

## 前提（重要）

- subnets
  - FargateProfile は private subnet のみを受け付けます。
  - BYO の `vpc-subnet-ids` は private subnet IDs を渡してください。
  - 検証用にネットワークが無い場合は、`docs/eks/byo-network/manifests/bootstrap-network-private-nat.yaml.tpl` で private subnets + NAT を ACK(EC2) で作れます。
- egress
  - private subnet の場合、NAT gateway か VPC endpoints が必要です（ECR pull / AWS APIs）。
- endpoint access
  - worker が join できるよう、EKS endpoint は `endpointPrivateAccess=true` を推奨します。
  - AccessEntry を使う場合は `authenticationMode=API_AND_CONFIG_MAP` を推奨します。
- security groups
  - BYO Topology 変数 `vpc-security-group-ids` は node 用 SG IDs として扱います。
  - `eks.kany8s.io/karpenter=enabled` かつ `vpc-security-group-ids=[]` の場合、`eks-karpenter-bootstrapper` が次を自動で行います。
    - ACK `ec2.services.k8s.aws/v1alpha1 SecurityGroup` を作成（subnet IDs から VPC ID/CIDR を discovery）
    - 作成された SG ID を `Cluster.spec.topology.variables["vpc-security-group-ids"]` に注入
    - ClusterClass の patch 経由で ACK EKS Cluster にも同じ SG IDs が伝播（control plane ENI と node で同一 SG を共有）
  - 既存 SG を使いたい場合は、`vpc-security-group-ids` に明示してください。

## 実装コンポーネント

- kubeconfig rotator（既存）
  - binary: `cmd/eks-kubeconfig-rotator/main.go`
  - manifests: `config/eks-plugin/`
  - docs: `docs/eks/plugin/eks-kubeconfig-rotator.md`
- karpenter bootstrapper（新規）
  - binary: `cmd/eks-karpenter-bootstrapper/main.go`
  - manifests: `config/eks-karpenter-bootstrapper/`

## セットアップ手順（MVP）

この README は “実装者向け” のため、詳細な手順は最小限です。

1) management cluster(kind) をセットアップ
  - `docs/eks/README.md`

2) BYO EKS ControlPlane を作る
  - `docs/eks/byo-network/README.md`

3) Flux を management cluster にインストール
  - `flux install`（source-controller + helm-controller が必要）
  - 目安: Flux >= v2.4（`source.toolkit.fluxcd.io/v1 OCIRepository` / `helm.toolkit.fluxcd.io/v2 HelmRelease` が必要）

```bash
flux install

kubectl api-resources --api-group=source.toolkit.fluxcd.io
kubectl api-resources --api-group=helm.toolkit.fluxcd.io
```

4) kubeconfig rotator をデプロイして有効化

```bash
export EKS_PLUGIN_IMG=example.com/eks-kubeconfig-rotator:dev
make docker-build-eks-plugin EKS_PLUGIN_IMG="$EKS_PLUGIN_IMG"
kind load docker-image "$EKS_PLUGIN_IMG" --name kany8s-eks
make deploy-eks-plugin EKS_PLUGIN_IMG="$EKS_PLUGIN_IMG"

kubectl -n "$NAMESPACE" annotate cluster "$CLUSTER_NAME" eks.kany8s.io/kubeconfig-rotator=enabled --overwrite
```

5) karpenter bootstrapper をデプロイして opt-in

```bash
export EKS_KARPENTER_BOOTSTRAPPER_IMG=example.com/eks-karpenter-bootstrapper:dev
make docker-build-eks-karpenter-bootstrapper EKS_KARPENTER_BOOTSTRAPPER_IMG="$EKS_KARPENTER_BOOTSTRAPPER_IMG"
kind load docker-image "$EKS_KARPENTER_BOOTSTRAPPER_IMG" --name kany8s-eks
make deploy-eks-karpenter-bootstrapper EKS_KARPENTER_BOOTSTRAPPER_IMG="$EKS_KARPENTER_BOOTSTRAPPER_IMG"

kubectl -n "$NAMESPACE" label cluster "$CLUSTER_NAME" eks.kany8s.io/karpenter=enabled --overwrite
```

6) 確認

- management cluster
  - `SecurityGroup` / `AccessEntry` / `FargateProfile` / `Role` / `Policy` / `InstanceProfile` / `OpenIDConnectProvider` が作られる
  - Flux の `HelmRelease` が作られる
- workload cluster
  - `karpenter` の Pod が Running（Fargate）
  - `kube-system/coredns` が Running（Fargate）
  - `NodePool` 適用後に node が増える
