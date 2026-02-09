# EKS Fargate bootstrap + Karpenter design (no NodeGroup)

- 作成日: 2026-02-07
- 更新日: 2026-02-08
- ステータス: MVP Implemented (design + implementation aligned)

## TL;DR

- EKS ControlPlane only の状態（Node=0）から Karpenter で worker を生やすために、まず `karpenter` と `coredns` を **EKS Fargate** で起動する（bootstrap compute）。
- そのために management cluster 側で ACK を使って以下を作る: OIDC Provider (IRSA) / Karpenter controller role / Karpenter node role / Fargate Pod execution role / FargateProfile。
- node join の許可は `aws-auth` ConfigMap ではなく、可能なら **EKS AccessEntry (type=EC2_LINUX)** で完結させる（workload へ触る箇所を減らす）。
- Karpenter のインストールは、management cluster の Flux `HelmRelease.spec.kubeConfig.secretRef` を使って **remote cluster** へ Helm install/upgrade する。
- `EC2NodeClass`/`NodePool` の適用は、現状のセットアップにある **ClusterResourceSet**（`addons.cluster.x-k8s.io/v1beta2`）で remote apply する。
- kubeconfig は `eks-kubeconfig-rotator` が必須（CAPI remote apply / RemoteConnectionProbe を安定化する）。
- Cleanup で IAM Role deletion が InstanceProfile 依存で詰まるのを避けるため、node 用 `InstanceProfile` を ACK で明示管理し、`EC2NodeClass.spec.instanceProfile` を使う（Karpenter に instance profile を自動生成させない）。

## Direction (no command ops)

最終的には「YAML apply + opt-in」だけで収束させ、`aws ...` や手動 `kubectl patch` のような command ops を不要にする。

- プラン: `docs/eks/fargate/plan.md`
- 実行ログ（現状の手作業を含む）: `docs/eks/fargate/wip.md`

## Background

`docs/eks/byo-network/` のサンプルは「既存 subnet IDs 上に EKS ControlPlane だけを作る」ことを目的にしており、worker は作らない。そのため次の状態が自然に起きる。

- `kubectl get nodes` は 0 件
- `kube-system/coredns` が Pending のまま

一方、NodeGroup なしで Karpenter を導入したい場合、Karpenter controller 自身が動作するための compute が必要になる。

CAPT の EKS workspace はこの bootstrap 問題を **Fargate profile** で解決している（Karpenter namespace を Fargate に載せる）。
本設計はその考え方を、Kany8s + kro + ACK ベースの EKS BYO サンプルに移植する。

関連ドキュメント:

- BYO network 設計方針: `docs/eks/byo-network/design.md`
- kubeconfig / RemoteConnectionProbe 対応: `docs/eks/plugin/eks-kubeconfig-rotator.md`
- kubeconfig contract: `docs/adr/0004-kubeconfig-secret-strategy.md`

## Goals

- worker(Node) が 0 の EKS からスタートして、Karpenter が稼働し Node を自動作成できる。
- “CAPI Cluster として” `RemoteConnectionProbe=True` を維持しつつ、workload に対しても検証できる状態にする。
- BYO network の削除事故（VPC/Subnet を消す）を起こさずに、クラスタ固有リソース（IAM/Fargate/Karpenter）だけを lifecycle 管理できる。

## Non-goals

- CAPI Machines / MachineDeployment / MachinePool として worker を管理する（今回は EKS + Karpenter で完結）。
- “完全自動” のネットワーク構築（NAT/VPC endpoints を含む）をこの設計で確定する。
  - ただし Fargate/Node が動くためのネットワーク要件は明記する。

## Constraints

- Kany8s core は provider-agnostic を維持し、EKS 固有の kubeconfig/IAM/Karpenter 連携は plugin/サンプル側に閉じ込める。
- BYO network は既存 VPC/Subnet を graph に含めない（`docs/eks/byo-network/design.md` の deletion safety）。
- 現状の management cluster には `HelmChartProxy` は無く、`ClusterResourceSet` のみが存在する。
  - `kubectl api-resources --api-group=addons.cluster.x-k8s.io` が `ClusterResourceSet(v1beta2)` を返す。

## Terminology / Name resolution

用語:

- `capiClusterName`: `Cluster.metadata.name`
- `controlPlaneName`: `Cluster.spec.controlPlaneRef.name`（Topology ではランダム suffix が付くことがある）
- `eksClusterName`: EKS 上の cluster 名
- `ackEKSClusterName`: ACK の `clusters.eks.services.k8s.aws` リソース名

この repo の BYO サンプルでは、ACK EKS Cluster は `controlPlaneName` で作成されるケースがあるため、`eksClusterName` の既定値は次を採用する。

- `eksClusterName = Cluster.metadata.annotations["eks.kany8s.io/cluster-name"] ?? (if controlPlaneRef.kind==Kany8sControlPlane then controlPlaneRef.name) ?? capiClusterName`

この解決ロジックは kubeconfig rotator と同じ（`docs/eks/plugin/eks-kubeconfig-rotator.md`）。

## High-level architecture

対象コンポーネント:

- Management cluster (kind)
  - CAPI core
  - kro
  - ACK (eks/iam/ec2)
  - Kany8s
  - `eks-kubeconfig-rotator` (既存)
  - (新規) `eks-karpenter-bootstrapper` (本設計)

- Workload cluster (EKS)
  - `kube-system/coredns` を Fargate で稼働
  - `karpenter/*` を Fargate で稼働
  - Karpenter が Node を起動

## Prerequisites (network)

### 1) FargateProfile の subnet 要件

ACK EKS `FargateProfile.spec.subnets` は **private subnet**（IGW への direct route が無い subnet）だけを受け付ける。
従って、この設計で指定する `vpc-subnet-ids` は原則 private subnet を想定する。

### 2) Egress 要件

Fargate と Karpenter node は少なくとも以下へ到達できる必要がある。

- container image pull（Karpenter controller image / CoreDNS image / kube-proxy / CNI 等）
- AWS APIs（EC2/IAM/EKS/SSM/Pricing 等）

private subnet の場合は次のいずれかが必要。

- NAT Gateway + route（一般的）
- VPC endpoints（ECR(api+dkr), S3, STS, EC2, SSM, CloudWatch Logs 等）

この設計では “既存 BYO network に上記が備わっている” 前提で進める（ネットワーク自動構築は別途）。

### 3) EKS endpoint access

`publicAccessCIDRs` を `/32` に絞ったまま Node を起動すると、Node が EKS API に到達できず join 失敗し得る。
Karpenter で worker を起動する前に、EKS control plane は次を推奨する。

- `endpointPrivateAccess: true`
- `endpointPublicAccess: true`
- `publicAccessCIDRs: [<management egress>/32]`（人間/management cluster 用）

BYO RGD `docs/eks/byo-network/manifests/eks-control-plane-byo-rgd.yaml` は現状 `endpointPrivateAccess:false` 固定なので、worker 対応としてパラメータ化する（後述）。

## AWS resources (created via ACK)

AWS リソースの create/update は、management cluster 上の ACK CustomResource で行う（= command ops を避ける）。

補足:

- kubeconfig の token 生成（`eks-kubeconfig-rotator`）は STS presign を使うため AWS API 相当の処理が入る。
- BYO の input だけでは不足するメタ情報（例: VPC ID）がある場合、read-only の discovery を追加で行う可能性がある（詳細は `docs/eks/fargate/plan.md`）。

### 1) OIDC Provider (IRSA)

Karpenter controller は IRSA を使う。

- kind: `iam.services.k8s.aws/v1alpha1 OpenIDConnectProvider`
- input:
  - `url`: `ACK EKS Cluster.status.identity.oidc.issuer`
  - `clientIDList`: `["sts.amazonaws.com"]`
  - `thumbprints`: issuer の SHA-1 thumbprint（任意）
    - (MVP) 省略し、AWS(IAM) 側に取得させる
    - (Optional) 明示設定する（Terraform の `tls_certificate` 相当の算出）

thumbprint を明示する場合の算出例（任意）:

```bash
# issuer host の TLS 証明書から SHA1 thumbprint を得る（例）
issuer_host="oidc.eks.ap-northeast-1.amazonaws.com"
issuer_path="/id/XXXXXXXX"

echo | openssl s_client -servername "${issuer_host}" -connect "${issuer_host}:443" 2>/dev/null \
  | openssl x509 -fingerprint -sha1 -noout \
  | sed -e 's/^SHA1 Fingerprint=//' -e 's/://g'
```

### 2) IAM roles

必要な role は 3 つ。

1) `KarpenterControllerRole` (IRSA)
  - trust: OIDC Provider + `system:serviceaccount:karpenter:karpenter`
  - policy: Karpenter controller policy（v1 系）
  - optional: interruption queue 読み取り

2) `KarpenterNodeRole` (EC2 instance role)
  - trust: `ec2.amazonaws.com`
  - managed policies (推奨):
    - `AmazonEKSWorkerNodePolicy`
    - `AmazonEKS_CNI_Policy`
    - `AmazonEC2ContainerRegistryReadOnly`
    - `AmazonSSMManagedInstanceCore` (任意)

3) `EKSFargatePodExecutionRole`
  - trust: `eks-fargate-pods.amazonaws.com`
  - managed policy:
    - `AmazonEKSFargatePodExecutionRolePolicy`

Karpenter controller policy は CAPT と同様に `terraform-aws-modules/eks//modules/karpenter` の v1 policy をベースにする。
（参考: `terraform-aws-modules/terraform-aws-eks` v20.29.0 `modules/karpenter/policy.tf` の `data.aws_iam_policy_document.v1`）

実装メモ（ACK）:

- managed policy だけでは不足するため、次のいずれかで “カスタム policy” を扱う。
  - `iam.services.k8s.aws/v1alpha1 Policy` を作成し、`Role.spec.policies` にその ARN を含める
  - もしくは Role の inline policy 機能（ACK が提供する場合）を使う

### 2.5) IAM InstanceProfile (Karpenter node)

Karpenter v1 の `EC2NodeClass` は `spec.role` か `spec.instanceProfile` のどちらかを要求する。

運用/cleanup の安定性のため、この設計では次を推奨する:

- node role は ACK `Role` で作成・管理する（既存方針のまま）
- node 用 `InstanceProfile` も ACK `InstanceProfile` で作成・管理する
- workload へ適用する `EC2NodeClass` は `spec.instanceProfile=<instance profile name>` を使う

狙い:

- Karpenter が instance profile を自動生成すると、クラスタ削除時に instance profile が残り
  ACK `Role` deletion が `DeleteConflict` で詰まることがある
- instance profile を ACK 管理下に置くことで、Kubernetes resource の deletion で収束させる

### 3) EKS AccessEntry (node join)

`aws-auth` ConfigMap を remote apply しなくても node join を成立させるため、EKS AccessEntry を使う。

- kind: `eks.services.k8s.aws/v1alpha1 AccessEntry`
- spec:
  - `clusterName`: `eksClusterName`
  - `principalARN`: `KarpenterNodeRole` の ARN
  - `type: EC2_LINUX`

注意:

- これを使うには EKS 側の access mode が `API` を含む必要がある（推奨: `API_AND_CONFIG_MAP`）。
- 現状の BYO RGD は `authenticationMode=CONFIG_MAP` のため、RGD で `API_AND_CONFIG_MAP` を明示する（後述）。

### 4) EKS FargateProfile

必要な profile は 2 つ。

- `karpenter`:
  - selectors: `{ namespace: "karpenter" }`
- `coredns`:
  - selectors: `{ namespace: "kube-system", labels: { "k8s-app": "kube-dns" } }`

どちらも `subnets` は BYO の private subnet IDs を使用し、`podExecutionRoleRef` は `EKSFargatePodExecutionRole` を参照する。

### 5) EC2 SecurityGroup (Karpenter nodes)

Karpenter が起動する EC2 node には security group が必要。

推奨方針（MVP / no command ops）:

- `vpc-security-group-ids` は “node 用 SG IDs” として扱う。
- `eks.kany8s.io/karpenter=enabled` かつ `vpc-security-group-ids` が空の場合、bootstrapper が node 用 SG を **CustomResource として**作成し、SG ID を `Cluster.spec.topology.variables["vpc-security-group-ids"]` へ注入して収束させる（手作業の `aws ec2 create-security-group` を排除）。
- 注入された SG IDs は次に使われる。
  - ACK EKS Cluster の `resourcesVPCConfig.securityGroupIDs`（control plane ENI に attach）
  - Karpenter の `EC2NodeClass.securityGroupSelectorTerms`（node に attach）

VPC ID の解決は BYO では追加情報が必要になるため、以下のどれかで成立させる（詳細は `docs/eks/fargate/plan.md`）。

- (入力) Topology variable で VPC ID を渡す
- (自動) subnet IDs から VPC ID を read-only discovery する
- (自動) bootstrap network（ACK で VPC/Subnet を作る）を併用し、VPC CR 参照で解決する

SG の最小要件（概略）:

- ingress:
  - node-to-node（同 SG 内）許可
  - control plane -> kubelet (TCP 10250)
- egress:
  - outbound all（ECR pull / EKS API / AWS APIs）

注意:

- BYO では VPC/SG の source of truth がクラスタ外になりやすい。必要な入力/自動解決の境界は `docs/eks/fargate/plan.md` にまとめる。

## Workload resources

### 1) Karpenter install

CAPT は Helm（HelmChartProxy）で Karpenter を入れているが、この repo では management cluster に Flux（source-controller/helm-controller）を入れ、MVP は Option C を主経路とする。

- Option A: Karpenter Helm chart を `helm template` で render した YAML を repo に vendoring し、ClusterResourceSet で apply
- Option B: 手元の `kubectl --kubeconfig <secret>` で手動 apply
- Option C (chosen): Flux の `HelmRelease` を使って Karpenter chart を install/upgrade する

補足:

- Karpenter は CRD を含むため、apply は “複数回で収束” を前提にする（初回は CRD 未登録で CustomResource の apply が落ち得る）。
- CAPT と同様に webhook は無効化し、まずは Fargate 上で確実に動く構成を優先する。

Karpenter の主要 values（CAPT と同型）:

```yaml
dnsPolicy: Default
priorityClassName: system-cluster-critical
settings:
  clusterName: <eksClusterName>
  clusterEndpoint: <https://...eks.amazonaws.com>
  # interruptionQueue: <queueName>  # optional
serviceAccount:
  annotations:
    eks.amazonaws.com/role-arn: <KarpenterControllerRoleArn>
webhook:
  enabled: false
```

Option C の補足（Flux）:

- Flux helm-controller は `HelmRelease.spec.kubeConfig.secretRef` で **remote cluster** に対して Helm を実行できる。
- その Secret は “毎 reconcile で読み直される” ため、EKS の短命 token も `eks-kubeconfig-rotator` でローテーションしていれば追従できる。
- これにより、workload cluster に Flux を常駐させずに、management cluster から Karpenter を install/upgrade できる。

リソース例（OCI chart + remote kubeconfig）:

```yaml
apiVersion: source.toolkit.fluxcd.io/v1
kind: OCIRepository
metadata:
  name: karpenter
  namespace: default
spec:
  interval: 10m
  url: oci://public.ecr.aws/karpenter/karpenter
  ref:
    tag: "1.0.8"  # example
  layerSelector:
    mediaType: application/vnd.cncf.helm.chart.content.v1.tar+gzip
    operation: copy
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: karpenter
  namespace: default
spec:
  interval: 10m
  releaseName: karpenter
  targetNamespace: karpenter
  kubeConfig:
    secretRef:
      name: <capiClusterName>-kubeconfig
  chartRef:
    kind: OCIRepository
    name: karpenter
    namespace: default
  install:
    createNamespace: true
    crds: CreateReplace
  upgrade:
    crds: CreateReplace
  values:
    dnsPolicy: Default
    priorityClassName: system-cluster-critical
    settings:
      clusterName: <eksClusterName>
      clusterEndpoint: <https://...eks.amazonaws.com>
    serviceAccount:
      annotations:
        eks.amazonaws.com/role-arn: <KarpenterControllerRoleArn>
    webhook:
      enabled: false
```

### 2) EC2NodeClass / NodePool

CAPT の `karpenter-aws-default-nodepool` 相当を、workload に `EC2NodeClass` と `NodePool` として適用する。

- `karpenter.k8s.aws/v1 EC2NodeClass`
  - `instanceProfile`: node 用 **instance profile name**（ACK `InstanceProfile.spec.name`）
  - `subnetSelectorTerms`: BYO subnet IDs を ID 指定（tag discovery には依存しない）
  - `securityGroupSelectorTerms`: 既存 SG IDs を ID 指定（もしくは node 用 SG を別途作成して ID 指定）

- `karpenter.sh/v1 NodePool`
  - `nodeClassRef` -> 上記 EC2NodeClass
  - `requirements`（instance category/arch/capacity-type 等）
  - `limits`（暴走防止の cpu 上限は推奨）

## Add-on delivery: ClusterResourceSet

### Why ClusterResourceSet

- 既に management cluster に導入済み（`addons.cluster.x-k8s.io/v1beta2 ClusterResourceSet`）。
- `eks-kubeconfig-rotator` により `<cluster>-kubeconfig` が常に有効で、remote apply が成立しやすい。

補足:

- Flux を採用する場合、Karpenter install は ClusterResourceSet ではなく Flux の `HelmRelease`（remote kubeconfig）へ寄せられる。
- この repo の実装では、ClusterResourceSet は `EC2NodeClass`/`NodePool` の apply にのみ使う。

### Rendering strategy

ClusterResourceSet はテンプレート展開を持たないため、cluster ごとに “render 済みマニフェスト” を用意する必要がある。

推奨:

- (新規) `eks-karpenter-bootstrapper` controller が以下を生成する。
  - `ConfigMap/<capiClusterName>-karpenter-nodepool`（EC2NodeClass/NodePool YAML）
  - `ClusterResourceSet/<capiClusterName>-karpenter-nodepool`（上記 ConfigMap を resources として参照）

ConfigMap の data 形式は ClusterResourceSet が解釈できる “YAML text” にする（例: `data.resources.yaml: |`）。この repo の実装は key=`resources.yaml` を使う。

opt-in は label を推奨（ClusterResourceSet selector が label のみのため）。

- `Cluster.metadata.labels["eks.kany8s.io/karpenter"] = "enabled"`

### Apply order

厳密な順序制御は難しいため、次の “収束” を前提にする。

1) ACK で FargateProfile が作成される
2) Flux HelmRelease が Karpenter を install/upgrade（CRD + controller）
3) ClusterResourceSet が EC2NodeClass/NodePool を apply（CRD 未登録でも re-apply で収束）

aws-auth を使う場合は ordering が重要になるため、可能なら AccessEntry を採用する（node join を workload 外で完結）。

## Proposed controller: `eks-karpenter-bootstrapper`

### Scope

- EKS 固有の “worker bootstrap (Fargate + Karpenter)” を management cluster 側で完結させる。
- Kany8s core は変更しない（plugin と manifests の追加で対応）。

### Watch / opt-in

- watch:
  - `cluster.x-k8s.io/v1beta2 Cluster`
  - `eks.services.k8s.aws/v1alpha1 Cluster`（ACK EKS Cluster）
  - `iam.services.k8s.aws/*`（Role/Policy/OIDC）
  - `eks.services.k8s.aws/*`（FargateProfile/AccessEntry）

- opt-in:
  - `Cluster.metadata.labels["eks.kany8s.io/karpenter"] = "enabled"`

### Reconcile flow (high-level)

1) 対象 Cluster を name 解決（`eksClusterName`）し、ACK EKS Cluster から `endpoint` と `oidc.issuer` を取得
2) OIDC provider を作成/待機（thumbprint は入力 or 自動取得）
3) IAM:
  - Karpenter controller policy/role
  - Karpenter node role
  - Fargate pod execution role
4) EKS:
  - AccessEntry (EC2_LINUX) で node join を許可
  - FargateProfile (karpenter/coredns)
5) Add-on apply:
  - (Flux) per-cluster `OCIRepository`/`HelmRelease` を生成し、remote kubeconfig で Karpenter を install/upgrade
  - (CAPI) per-cluster ConfigMap + ClusterResourceSet を生成し、workload へ EC2NodeClass/NodePool を適用
6) 収束確認:
  - `karpenter` pod が Running になり、pending workload があれば node が増える

## Changes to existing manifests (BYO RGD)

### 1) `eks-control-plane-byo.kro.run` の endpoint 設定をパラメータ化

`docs/eks/byo-network/manifests/eks-control-plane-byo-rgd.yaml` に以下を追加する。

- `spec.vpc.endpointPrivateAccess` (bool, default=true)
- `spec.vpc.endpointPublicAccess` (bool, default=true)
- `spec.access.authenticationMode` (string, default="API_AND_CONFIG_MAP")

そして ACK EKS Cluster へ反映する。

- `Cluster.spec.accessConfig.authenticationMode = <authenticationMode>`
- `Cluster.spec.resourcesVPCConfig.endpointPrivateAccess = <endpointPrivateAccess>`
- `Cluster.spec.resourcesVPCConfig.endpointPublicAccess = <endpointPublicAccess>`

加えて、AccessEntry を使う前提なら `Cluster.spec.accessConfig.authenticationMode` は `API_AND_CONFIG_MAP` を推奨する。

### 2) ClusterClass variables

BYO の Topology variables に以下を追加する（例）。

- `eks-access-mode`（default: `API_AND_CONFIG_MAP`）
- `eks-endpoint-private-access`（default: true）
- `eks-endpoint-public-access`（default: true）

## Failure modes / Troubleshooting

- Karpenter pod が Pending のまま
  - FargateProfile が無い / selectors が一致しない / subnet が private ではない

- Karpenter pod が ImagePullBackOff
  - private subnet に NAT/VPC endpoints が無い

- Node が起動するが join しない
  - endpointPrivateAccess が無い + publicAccessCIDRs が絞られている
  - AccessEntry/aws-auth が無い（node role が認可されていない）
  - Node role に必要な managed policy が足りない

- Karpenter controller が AWS API で 403
  - IRSA の OIDC provider / thumbprint / trust policy / serviceAccount annotation の不整合
  - controller policy 不足（v1 policy を使う）

## Test plan (manual)

最低限の成功条件:

- workload で `karpenter` namespace の pod が Running（Fargate 上）
- workload で `NodePool` を作ると `kubectl get nodes` に node が現れる

確認例（workload kubeconfig は `<capiClusterName>-kubeconfig` を利用）:

```bash
kubectl --kubeconfig /tmp/<cluster>.kubeconfig -n karpenter get pods
kubectl --kubeconfig /tmp/<cluster>.kubeconfig get nodepool,ec2nodeclass
kubectl --kubeconfig /tmp/<cluster>.kubeconfig get nodes -o wide
```

## Cleanup

- cluster deletion 前に、ACK finalizer が走れるよう management cluster を先に消さない（`docs/eks/cleanup.md` と同じ）。
- BYO network（VPC/Subnet）は残るのが正しい挙動。
- 追加で作った IAM/OIDC/FargateProfile はクラスタ固有リソースとして削除対象。
- node 用 InstanceProfile もクラスタ固有リソースとして削除対象（ACK 管理下に置く）。

## References

- CAPT (Karpenter install strategy): https://github.com/appthrust/capt
  - CAPTEP-0030: https://raw.githubusercontent.com/appthrust/capt/main/docs/CAPTEP/0030-karpenter-installation-reliability.md
  - CAPTEP-0043: https://raw.githubusercontent.com/appthrust/capt/main/docs/CAPTEP/0043-karpenter-helm-chart-proxy-migration.md
- Terraform EKS module (Karpenter v1 policy baseline): https://raw.githubusercontent.com/terraform-aws-modules/terraform-aws-eks/v20.29.0/modules/karpenter/policy.tf
- Karpenter docs: https://karpenter.sh/docs/
