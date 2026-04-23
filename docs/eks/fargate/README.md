# EKS: Fargate bootstrap + Karpenter (NodeGroupなし)

このディレクトリは `docs/eks/byo-network/` の BYO EKS ControlPlane に対して、

- `kube-system/coredns` と `karpenter` を EKS Fargate で起動（ノード0でも起動可能にする）
- Karpenter で EC2 worker を自動作成

するための設計/実装メモです。

## 読む順番

- プラン: `docs/eks/fargate/plan.md`
- 設計: `docs/eks/fargate/design.md`
- 実装 TODO: `docs/eks/fargate/todo.md`
- Flux versioning: `docs/eks/fargate/flux-upgrade.md`
- break-glass: `docs/eks/fargate/break-glass.md`
- 実行ログ: `docs/eks/fargate/wip.md`

## 前提（重要）

- subnets
  - FargateProfile は private subnet のみを受け付けます。
  - BYO の subnet 変数は用途で 2 種類に分かれています:
    - `vpc-control-plane-subnet-ids` (EKS control plane ENI 用): NAT egress 不要。endpoint アクセスモードに応じて public/private/isolated を選択。
    - `vpc-node-subnet-ids` (FargateProfile + Karpenter NodePool 用): private subnet + NAT default route が必須(image pull のため)。
  - 検証用にネットワークが無い場合は、`docs/eks/byo-network/manifests/bootstrap-network-private-nat.yaml.tpl` で private subnets + NAT を ACK(EC2) で作れます。
- egress
  - private subnet の場合、NAT gateway か VPC endpoints が必要です（ECR pull / AWS APIs）。
- endpoint access
  - worker が join できるよう、EKS endpoint は `endpointPrivateAccess=true` を推奨します。
  - AccessEntry を使う場合は `authenticationMode=API_AND_CONFIG_MAP` を推奨します。
- security groups
  - BYO Topology 変数 `vpc-node-security-group-ids`（任意）を node 用 SG IDs として優先します。
  - `vpc-node-security-group-ids` が未指定の場合は、後方互換として `vpc-security-group-ids` を使います。
  - `eks.kany8s.io/karpenter=enabled` かつ node 用 SG IDs が `[]` の場合、`eks-karpenter-bootstrapper` が次を自動で行います。
    - ACK `ec2.services.k8s.aws/v1alpha1 SecurityGroup` を作成（subnet IDs から VPC ID/CIDR を discovery）
    - 作成された SG ID を `Cluster.spec.topology.variables["vpc-node-security-group-ids"]` に注入
    - control plane 側 (`vpc-security-group-ids`) が空なら同じ SG ID を補完
  - 既存 SG を使いたい場合は `vpc-node-security-group-ids`（または後方互換で `vpc-security-group-ids`）に明示してください。
- takeover policy
  - デフォルトでは、plugin は unmanaged な Secret/ConfigMap/CR を上書きしません。
  - 明示 takeover が必要な場合は `Cluster` に `eks.kany8s.io/allow-unmanaged-takeover=enabled` を付与してください。
- interruption handling (optional)
  - Karpenter interruption queue を使う場合は、`eks.kany8s.io/karpenter-interruption-queue=<queue-name>` を `Cluster` に付与します。
  - 現在の実装スコープは `settings.interruptionQueue` 注入のみです。SQS/EventBridge の作成・配線と controller role の SQS 権限付与は別途実施してください。
- OIDC thumbprint (optional)
  - `eks.kany8s.io/oidc-thumbprint-auto=enabled` を `Cluster` に付与すると、bootstrapper が issuer から thumbprint を算出して `OpenIDConnectProvider.spec.thumbprints` を設定します。
  - 算出対象は root ではなく top intermediate CA thumbprint（AWS IAM 要件）です。
  - 証明書チェーンが検証できない場合は thumbprint 設定を skip し、Warning Event を出して reconcile は継続します。

## 実装コンポーネント

- kubeconfig rotator（既存）
  - binary: `cmd/eks-kubeconfig-rotator/main.go`
  - manifests:
    - base: `config/eks-plugin/`
    - kind overlay: `config/overlays/eks-plugin/kind/`
    - IRSA overlay: `config/overlays/eks-plugin/irsa/`
  - docs: `docs/eks/plugin/eks-kubeconfig-rotator.md`
- karpenter bootstrapper（新規）
  - binary: `cmd/eks-karpenter-bootstrapper/main.go`
  - manifests:
    - base: `config/eks-karpenter-bootstrapper/`
    - kind overlay: `config/overlays/eks-karpenter-bootstrapper/kind/`
    - IRSA overlay: `config/overlays/eks-karpenter-bootstrapper/irsa/`

## Credentials strategy

ACK controller と同じ shared credentials file 方式に揃えています (`aws.credentials.secretName` が非空なら Secret を mount し、空なら SDK default chain)。

- kind 管理クラスタ
  - `ack-system/aws-creds` Secret を `/var/run/secrets/aws/credentials` に mount する overlay を使います（ACK 本家と同じ path）。
  - 固定前提:
    - namespace: `ack-system`
    - secret name: `aws-creds`
    - env: `AWS_SHARED_CREDENTIALS_FILE=/var/run/secrets/aws/credentials`, `AWS_PROFILE=default`
- 実クラスタ
  - IRSA overlay を使い、`aws-creds` mount を無効化します。
  - ServiceAccount annotation (`eks.amazonaws.com/role-arn`) は環境に合わせて付与してください。

## セットアップ手順（MVP）

この README は “実装者向け” のため、詳細な手順は最小限です。

1) management cluster(kind) をセットアップ
  - `docs/eks/README.md`

2) BYO EKS ControlPlane を作る
  - `docs/eks/byo-network/README.md`

3) Flux を management cluster にインストール
  - pin: `v2.4.0`（`source.toolkit.fluxcd.io/v1 OCIRepository` / `helm.toolkit.fluxcd.io/v2 HelmRelease` が必要）
  - runbook: `docs/eks/fargate/flux-upgrade.md`

```bash
export FLUX_VERSION=v2.4.0
bash hack/eks-install-flux.sh

kubectl api-resources --api-group=source.toolkit.fluxcd.io
kubectl api-resources --api-group=helm.toolkit.fluxcd.io
```

4) kubeconfig rotator をデプロイして有効化

```bash
export EKS_PLUGIN_IMG=example.com/eks-kubeconfig-rotator:dev
make docker-build-eks-plugin EKS_PLUGIN_IMG="$EKS_PLUGIN_IMG"
kind load docker-image "$EKS_PLUGIN_IMG" --name kany8s-eks
make deploy-eks-plugin EKS_PLUGIN_IMG="$EKS_PLUGIN_IMG"

# 明示的に kustomize を使う場合（kind）
kubectl apply -k config/overlays/eks-plugin/kind

kubectl -n "$NAMESPACE" annotate cluster "$CLUSTER_NAME" eks.kany8s.io/kubeconfig-rotator=enabled --overwrite
```

5) karpenter bootstrapper をデプロイして opt-in

```bash
export EKS_KARPENTER_BOOTSTRAPPER_IMG=example.com/eks-karpenter-bootstrapper:dev
make docker-build-eks-karpenter-bootstrapper EKS_KARPENTER_BOOTSTRAPPER_IMG="$EKS_KARPENTER_BOOTSTRAPPER_IMG"
kind load docker-image "$EKS_KARPENTER_BOOTSTRAPPER_IMG" --name kany8s-eks
make deploy-eks-karpenter-bootstrapper EKS_KARPENTER_BOOTSTRAPPER_IMG="$EKS_KARPENTER_BOOTSTRAPPER_IMG"

# 明示的に kustomize を使う場合（kind）
kubectl apply -k config/overlays/eks-karpenter-bootstrapper/kind

kubectl -n "$NAMESPACE" label cluster "$CLUSTER_NAME" eks.kany8s.io/karpenter=enabled --overwrite

# (任意) chart version override
kubectl -n "$NAMESPACE" annotate cluster "$CLUSTER_NAME" eks.kany8s.io/karpenter-chart-version=1.0.8 --overwrite

# (任意) HelmRelease values override (JSON object)
kubectl -n "$NAMESPACE" annotate cluster "$CLUSTER_NAME" \
  eks.kany8s.io/karpenter-helm-values-override-json='{"resources":{"requests":{"cpu":"500m","memory":"512Mi"}}}' \
  --overwrite

# (任意) NodePool/EC2NodeClass template override
# ConfigMap data[resources.yaml] を使って差し替える
kubectl -n "$NAMESPACE" annotate cluster "$CLUSTER_NAME" \
  eks.kany8s.io/karpenter-nodepool-template-configmap=my-nodepool-template \
  --overwrite

# (任意) interruption handling queue
kubectl -n "$NAMESPACE" annotate cluster "$CLUSTER_NAME" \
  eks.kany8s.io/karpenter-interruption-queue=my-karpenter-interruption-queue \
  --overwrite

# (任意) issuer から OIDC thumbprint を自動算出して OIDCProvider に設定
kubectl -n "$NAMESPACE" annotate cluster "$CLUSTER_NAME" \
  eks.kany8s.io/oidc-thumbprint-auto=enabled \
  --overwrite

# (任意) unmanaged Secret/CR/ConfigMap を明示 takeover
kubectl -n "$NAMESPACE" annotate cluster "$CLUSTER_NAME" \
  eks.kany8s.io/allow-unmanaged-takeover=enabled \
  --overwrite
```

6) 確認

- management cluster
  - `SecurityGroup` / `AccessEntry` / `FargateProfile` / `Role` / `Policy` / `InstanceProfile` / `OpenIDConnectProvider` が作られる
  - Flux の `HelmRelease` が作られる
- workload cluster
  - `karpenter` の Pod が Running（Fargate）
  - `kube-system/coredns` が Running（Fargate）
  - `NodePool` 適用後に node が増える

## 観測コマンド（plugin-managed）

```bash
# Kubernetes resources (label-based)
kubectl -n "$NAMESPACE" get \
  openidconnectproviders.iam.services.k8s.aws,roles.iam.services.k8s.aws,policies.iam.services.k8s.aws,instanceprofiles.iam.services.k8s.aws,\
  accessentries.eks.services.k8s.aws,fargateprofiles.eks.services.k8s.aws,securitygroups.ec2.services.k8s.aws,\
  ocirepositories.source.toolkit.fluxcd.io,helmreleases.helm.toolkit.fluxcd.io,\
  configmaps,secrets,clusterresourcesets.addons.cluster.x-k8s.io \
  -l cluster.x-k8s.io/cluster-name="$CLUSTER_NAME" -o wide

# EC2 instances created by Karpenter (AWS tag-based)
aws ec2 describe-instances \
  --region "$AWS_REGION" \
  --filters "Name=tag:karpenter.sh/discovery,Values=$EKS_CLUSTER_NAME" \
            "Name=instance-state-name,Values=pending,running,stopping,stopped,shutting-down"
```

## 再現スクリプト

BYO cluster 作成済みの前提で、`plugins deploy + opt-in + node join` までをまとめて実行するスクリプト:

```bash
export NAMESPACE=default
export CLUSTER_NAME=<your-cluster-name>
bash hack/eks-fargate-byo-node-join.sh
```

## Plugin deployment 方針

`eks-kubeconfig-rotator` と `eks-karpenter-bootstrapper` は現時点では **2 deployment のまま維持** します。

- 利点: RBAC/資格情報の最小化、障害分離、段階ロールアウトが容易
- 欠点: deployment が2つになり運用対象が増える
- 将来方針: 単一 binary/deployment への統合は optional（必要時に再評価）

## 削除範囲マトリクス

| 項目 | BYO network | bootstrap-network-private-nat |
|---|---|---|
| EKS Cluster / IAM / AccessEntry / FargateProfile / Flux / ClusterResourceSet / node SG / Karpenter EC2 | 削除対象 | 削除対象 |
| VPC / Subnet / NAT / RouteTable / IGW | 削除対象外（既存を維持） | 削除対象（ACKで作成したもの） |

## Break-glass

- 通常は `hack/eks-fargate-dev-reset.sh` を使ってください。
- 最後の手段（手動削除）は `docs/eks/fargate/break-glass.md` を参照してください。
