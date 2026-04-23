# Step 0 values (EKS smoke / BYO network)

`docs/eks/README.md` の環境変数を決めるためのメモです。

smoke test は `docs/eks/manifests/eks-control-plane-smoke-rgd.yaml` (Parent RGD) で
VPC/Subnet を ACK(EC2) で作り、同じ kro graph の中で EKS に配線します。
そのため既存の Subnet ID (`subnet-xxxx`) を事前に用意する必要はありません。

BYO network は `docs/eks/byo-network/manifests/*` を使い、既存の subnet IDs を `Cluster.spec.topology.variables` に渡します。

## 何を決めるか

最低限これを決めます:

```bash
export AWS_REGION=ap-northeast-1
export CLUSTER_NAME=demo-eks-135-$(date +%Y%m%d%H%M%S)
export NAMESPACE=default

# smoke: EKS の version は "1.xx" 形式 (例: 1.35)
export KUBERNETES_VERSION=1.35

# 新規作成する VPC/Subnet の CIDR と AZ
export VPC_CIDR=10.35.0.0/16
export SUBNET_A_CIDR=10.35.0.0/24
export SUBNET_A_AZ=ap-northeast-1a
export SUBNET_B_CIDR=10.35.1.0/24
export SUBNET_B_AZ=ap-northeast-1c

# (推奨) EKS public endpoint を許可する CIDR
# - 検証でも可能なら自分のグローバルIP(/32)に絞ってください。
# - あえて全開放するなら "0.0.0.0/0" を指定します(非推奨)。
export PUBLIC_ACCESS_CIDR="$(curl -fsSL https://checkip.amazonaws.com | tr -d '\n')/32"
```

BYO network で追加で必要な値:

```bash
# 既存 subnet IDs
# - CONTROL_PLANE_SUBNET_ID_*: EKS control plane ENI 用。AWS API 要件で >=2 across >=2 AZ 必須。NAT egress は不要 (class は endpoint access mode に依存)。
# - NODE_SUBNET_ID_*: karpenter Fargate + 既定 EC2NodeClass 用。private + NAT egress 必須。AWS API としては 1 subnet で動くが、HA のため >=2 AZ 推奨。
export CONTROL_PLANE_SUBNET_ID_1=subnet-aaaa1111
export CONTROL_PLANE_SUBNET_ID_2=subnet-bbbb2222
export NODE_SUBNET_ID_1=subnet-cccc3333
export NODE_SUBNET_ID_2=subnet-dddd4444

# security group IDs
# - `vpc-security-group-ids` (control plane向け) は従来どおり required です。
# - Fargate + Karpenter bootstrap では、node向けに `vpc-node-security-group-ids` を優先します。
#   - 未指定時は `vpc-security-group-ids` を後方互換として利用します。
#   - 手作業を避けたい場合は `[]` のままでも OK です。
#     - `eks-karpenter-bootstrapper` が node 用 SG を ACK で自動作成し、`vpc-node-security-group-ids` に注入します。
#     - control plane 側が空なら `vpc-security-group-ids` にも同じ SG を補完します。
#     - この動作には management cluster に ACK EC2(SecurityGroup) が導入済みであることと、
#       bootstrapper の AWS credentials で `DescribeSubnets/DescribeRouteTables/DescribeVpcs` が可能であることが必要です。
export SECURITY_GROUP_IDS_JSON='[]'
export NODE_SECURITY_GROUP_IDS_JSON='[]'

# (任意) Karpenter node role へ追加する managed policy ARNs
# 例: ["arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"]
export KARPENTER_NODE_ADDITIONAL_POLICY_ARNS_JSON='[]'

# BYO (Topology):
# - Cluster.spec.topology.version は semver が必須 (例: v1.35.0)
# - EKS 自体は major.minor (例: 1.35)
export KUBERNETES_VERSION=v1.35.0
export EKS_VERSION=1.35

# AccessEntry を使う前提の推奨値
export EKS_ACCESS_MODE=API_AND_CONFIG_MAP

# worker node の join 失敗を避けるため、private/public の併用を推奨
export EKS_ENDPOINT_PRIVATE_ACCESS=true
export EKS_ENDPOINT_PUBLIC_ACCESS=true
```

補足:

- `CLUSTER_NAME` は Kubernetes の object 名にもなるので、基本は `lowercase + '-'` で `<= 63` を推奨します。
- `SUBNET_A_AZ` と `SUBNET_B_AZ` は別 AZ にしてください（EKS の要件）。
- `SUBNET_*_CIDR` は `VPC_CIDR` の範囲内に収め、互いに重ならないようにしてください。

## 値の決め方

### 1) `AWS_REGION`

既に AWS CLI の default region を持っているならそれを使えます:

```bash
aws configure get region
```

明示する場合:

```bash
export AWS_REGION=ap-northeast-1
aws sts get-caller-identity
```

### 2) `SUBNET_A_AZ` / `SUBNET_B_AZ`

まず AZ 一覧を出します:

```bash
aws ec2 describe-availability-zones --region "$AWS_REGION" \
  --query 'AvailabilityZones[?State==`available`].[ZoneName,ZoneId]' \
  --output table
```

この中から同一 region の別 AZ を 2 つ選びます（例: `ap-northeast-1a` と `ap-northeast-1c`）。

### 3) `VPC_CIDR` / `SUBNET_*_CIDR`

検証用途なら次のようなシンプルな割り当てがおすすめです:

- `VPC_CIDR=10.35.0.0/16`
- `SUBNET_A_CIDR=10.35.0.0/24`
- `SUBNET_B_CIDR=10.35.1.0/24`

既存のネットワークとピアリングする等が無い限り、このままで問題になりにくいです。

### 4) `KUBERNETES_VERSION`

EKS がサポートしている version の中から選びます（例: `1.35`）。

手元でざっくり確認する例:

```bash
aws eks describe-addon-versions --region "$AWS_REGION" --addon-name kube-proxy \
  --query 'addons[0].addonVersions[].compatibilities[].clusterVersion' --output text \
  | tr '\t' '\n' | sort -u
```

### 5) (推奨) endpoint の公開 CIDR を絞る

この smoke test の RGD は `publicAccessCIDRs` を持っていて、指定しないと `0.0.0.0/0` になります。

テストでも可能なら `/32` に絞ってください:

```bash
my_ip="$(curl -fsSL https://checkip.amazonaws.com | tr -d '\n')"
export PUBLIC_ACCESS_CIDR="${my_ip}/32"
```

反映方法は `docs/eks/README.md` Step 8 のテンプレ render コマンドで
`PUBLIC_ACCESS_CIDR` を `publicAccessCIDRs` に注入します。

BYO network では `eks.publicAccessCIDRs` が ClusterClass 変数として required です。
`PUBLIC_ACCESS_CIDR` は省略せず、明示的に設定してください。

## BYO: Topology variables への対応

`docs/eks/byo-network/manifests/clusterclass-eks-byo.yaml` の変数と環境変数の対応例:

- `region` <- `AWS_REGION`
- `eks-version` <- `EKS_VERSION`
- `vpc-control-plane-subnet-ids` <- `CONTROL_PLANE_SUBNET_ID_1`, `CONTROL_PLANE_SUBNET_ID_2`
  - control plane ENI placement only (no NAT egress required; class depends on `eks-endpoint-*-access` mode)
- `vpc-node-subnet-ids` <- `NODE_SUBNET_ID_1` (+ optional `NODE_SUBNET_ID_2`)
  - karpenter Fargate profile + default EC2NodeClass `subnetSelectorTerms`; must be private with NAT default route. `>=1` subnet 必須 (AWS FargateProfile は 1 subnet を許容)。HA のため `>=2` AZ 推奨 (AZ 数 < 2 で controller が warning event を発火するが reconcile はブロックしない)
- `vpc-security-group-ids` <- `SECURITY_GROUP_IDS_JSON`
- `vpc-node-security-group-ids` <- `NODE_SECURITY_GROUP_IDS_JSON` (optional; node向け)
- `karpenter-node-role-additional-policy-arns` <- `KARPENTER_NODE_ADDITIONAL_POLICY_ARNS_JSON` (optional)
- `eks-public-access-cidrs` <- `PUBLIC_ACCESS_CIDR`
- `eks-access-mode` <- `EKS_ACCESS_MODE` (default: `API_AND_CONFIG_MAP`)
- `eks-endpoint-private-access` <- `EKS_ENDPOINT_PRIVATE_ACCESS` (default: `true`)
- `eks-endpoint-public-access` <- `EKS_ENDPOINT_PUBLIC_ACCESS` (default: `true`)

namespace 注意:

- `docs/eks/byo-network/manifests/clusterclass-eks-byo.yaml` は template の `metadata.namespace` を固定していません。
- `kubectl -n "$NAMESPACE" apply -f docs/eks/byo-network/manifests/clusterclass-eks-byo.yaml` のように、対象 `Cluster` と同じ namespace に apply してください。

`docs/eks/byo-network/manifests/cluster.yaml.tpl` の render で置換する値:

```bash
__AWS_REGION__                    -> ${AWS_REGION}
__CONTROL_PLANE_SUBNET_ID_1__     -> ${CONTROL_PLANE_SUBNET_ID_1}
__CONTROL_PLANE_SUBNET_ID_2__     -> ${CONTROL_PLANE_SUBNET_ID_2}
__NODE_SUBNET_ID_1__              -> ${NODE_SUBNET_ID_1}
__NODE_SUBNET_ID_2__              -> ${NODE_SUBNET_ID_2}
__SECURITY_GROUP_IDS_JSON__       -> ${SECURITY_GROUP_IDS_JSON}
__PUBLIC_ACCESS_CIDR__       -> ${PUBLIC_ACCESS_CIDR}
__EKS_ACCESS_MODE__          -> ${EKS_ACCESS_MODE}
__EKS_ENDPOINT_PRIVATE_ACCESS__ -> ${EKS_ENDPOINT_PRIVATE_ACCESS}
__EKS_ENDPOINT_PUBLIC_ACCESS__  -> ${EKS_ENDPOINT_PUBLIC_ACCESS}
```

## 最終的に貼る値 (テンプレ)

```bash
export AWS_REGION=
export CLUSTER_NAME=
export NAMESPACE=default

export KUBERNETES_VERSION=

export PUBLIC_ACCESS_CIDR=

export VPC_CIDR=
export SUBNET_A_CIDR=
export SUBNET_A_AZ=
export SUBNET_B_CIDR=
export SUBNET_B_AZ=

export CONTROL_PLANE_SUBNET_ID_1=
export CONTROL_PLANE_SUBNET_ID_2=
export NODE_SUBNET_ID_1=
export NODE_SUBNET_ID_2=

# Fargate + Karpenter bootstrap を使う場合は [] ではなく node 用 SG IDs を入れてください。
export SECURITY_GROUP_IDS_JSON='[]'
export NODE_SECURITY_GROUP_IDS_JSON='[]'
export KARPENTER_NODE_ADDITIONAL_POLICY_ARNS_JSON='[]'
export EKS_ACCESS_MODE=API_AND_CONFIG_MAP
export EKS_ENDPOINT_PRIVATE_ACCESS=true
export EKS_ENDPOINT_PUBLIC_ACCESS=true
```
