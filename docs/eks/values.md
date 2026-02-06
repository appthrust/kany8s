# Step 0 values (EKS smoke test)

`docs/eks/README.md` の Step 0 で設定する値を決めるためのメモです。

この smoke test は `docs/eks/manifests/eks-control-plane-smoke-rgd.yaml` (Parent RGD) で
VPC/Subnet を ACK(EC2) で作り、同じ kro graph の中で EKS に配線します。
そのため既存の Subnet ID (`subnet-xxxx`) を事前に用意する必要はありません。

## 何を決めるか

最低限これを決めます:

```bash
export AWS_REGION=ap-northeast-1
export CLUSTER_NAME=demo-eks-135-$(date +%Y%m%d%H%M%S)
export NAMESPACE=default

# EKS の version は "1.xx" 形式 (例: 1.35)
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
```
