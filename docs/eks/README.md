# EKS smoke test (kro + AWS ACK)

このドキュメントは、kind 上の管理クラスタから `Kany8sControlPlane` (kro backend) + kro + AWS ACK を使って、実際に AWS に EKS クラスタ(Control Plane)を作る手順です。

この手順で作られる EKS は「Control Plane のみ」です（NodeGroup は作りません）。

## 目的 (この手順で確認すること)

- AWS 側で EKS Cluster が作成され `ACTIVE` になる
- kro instance の正規化 status (`ready/endpoint/reason/message`) が `Kany8sControlPlane` に反映される
- (任意) CAPI `Cluster` を作って、owner `Cluster` 解決/label 注入などの facade 側ロジックが動く

## BYO network (既存 VPC/Subnet を使う場合)

既存の VPC/Subnet を使って EKS Control Plane のみを作成する場合は、smoke 用 (`*-smoke-*`) ではなく BYO 用 manifest を使ってください。

- サンプル: `docs/eks/byo-network/README.md`
- 設計: `docs/eks/byo-network/design.md`
- infra input-gate RGD: `docs/eks/byo-network/manifests/aws-byo-network-rgd.yaml`
- control plane RGD: `docs/eks/byo-network/manifests/eks-control-plane-byo-rgd.yaml`
- ClusterClass: `docs/eks/byo-network/manifests/clusterclass-eks-byo.yaml`
- Topology Cluster template: `docs/eks/byo-network/manifests/cluster.yaml.tpl`

最短 apply 手順（Step 1-6 で kind/CAPI/kro/ACK/Kany8s を入れた後）:

```bash
kubectl apply -f docs/eks/byo-network/manifests/aws-byo-network-rgd.yaml
kubectl wait --for=condition=ResourceGraphAccepted --timeout=120s rgd/aws-byo-network.kro.run

kubectl apply -f docs/eks/byo-network/manifests/eks-control-plane-byo-rgd.yaml
kubectl wait --for=condition=ResourceGraphAccepted --timeout=120s rgd/eks-control-plane-byo.kro.run

# ClusterClass + Template は Cluster と同じ namespace へ apply する
kubectl -n "$NAMESPACE" apply -f docs/eks/byo-network/manifests/clusterclass-eks-byo.yaml
```

BYO Topology Cluster の render/apply 例:

```bash
export SUBNET_ID_1=subnet-aaaa1111
export SUBNET_ID_2=subnet-bbbb2222
export SECURITY_GROUP_IDS_JSON='[]' # 例: '["sg-xxxx","sg-yyyy"]'

# CAPI Topology は semver が必須 (例: v1.35.0)
# EKS 自体は major.minor 形式 (例: 1.35)
export EKS_VERSION=1.35

rendered=/tmp/eks-cluster-byo.yaml
sed \
  -e "s|__CLUSTER_NAME__|${CLUSTER_NAME}|g" \
  -e "s|__NAMESPACE__|${NAMESPACE}|g" \
  -e "s|__KUBERNETES_VERSION__|${KUBERNETES_VERSION}|g" \
  -e "s|__AWS_REGION__|${AWS_REGION}|g" \
  -e "s|__EKS_VERSION__|${EKS_VERSION}|g" \
  -e "s|__SUBNET_ID_1__|${SUBNET_ID_1}|g" \
  -e "s|__SUBNET_ID_2__|${SUBNET_ID_2}|g" \
  -e "s|__SECURITY_GROUP_IDS_JSON__|${SECURITY_GROUP_IDS_JSON}|g" \
  -e "s|__PUBLIC_ACCESS_CIDR__|${PUBLIC_ACCESS_CIDR}|g" \
  docs/eks/byo-network/manifests/cluster.yaml.tpl > "${rendered}"

kubectl apply -f "${rendered}"
```

BYO 進捗確認例:

```bash
# BYO の kro instance リソース名を確認
kubectl api-resources --api-group=kro.run | grep -E 'awsbyonetwork|ekscontrolplane' || true

# BYO infra input-gate
kubectl -n "$NAMESPACE" get awsbyonetworks.kro.run "$CLUSTER_NAME" -o yaml || true

# BYO control plane
kubectl -n "$NAMESPACE" get ekscontrolplanebyos.kro.run "$CLUSTER_NAME" -o yaml || true
```

BYO フローでは既存 VPC/Subnet は管理対象に入れません。Cleanup でも VPC/Subnet は削除されません（`docs/eks/cleanup.md` 参照）。

## 注意 (コスト/安全)

- EKS control plane は課金対象です。検証後は必ず削除してください。
- kind 管理クラスタを先に消すと、ACK の finalizer による削除ができず AWS リソースが残ることがあります。
  - 先に Kubernetes リソースを削除し、ACK が AWS リソース削除を完了したことを確認してから kind を削除してください。
- この手順のデフォルトは EKS API endpoint の Public access を許可します（CIDR は `0.0.0.0/0`）。
  - 本番では必ず制限してください。検証でも可能なら自分のグローバル IP に絞ってください。

## 前提

- ツール
  - `docker`, `kind`, `kubectl`, `helm` (3.8+), `aws`, `jq`, `curl`, `make`, `go`
  - (任意) `clusterctl` (CAPI `Cluster` も apply する場合)

- AWS
  - `aws sts get-caller-identity` が通ること
  - 作成/削除に必要な権限があること (検証用なら Administrator 相当を推奨)
  - EC2(VPC/Subnet) + IAM + EKS を作成/削除できること
  - 利用可能な AZ が 2 つ以上あること (EKS は異なる AZ の Subnet を 2 つ以上要求)

## 0) 環境変数

値の決め方は `docs/eks/values.md` を参照してください。

この smoke test は VPC/Subnet も含めて AWS に新規作成します（Cleanup で削除されます）。

最低限:

```bash
export AWS_REGION=ap-northeast-1
export CLUSTER_NAME=demo-eks-135-$(date +%Y%m%d%H%M%S)
export NAMESPACE=default

# EKS version は "1.xx" 形式 (例: 1.35)
export KUBERNETES_VERSION=1.35

export VPC_CIDR=10.35.0.0/16
export SUBNET_A_CIDR=10.35.0.0/24
export SUBNET_A_AZ=ap-northeast-1a
export SUBNET_B_CIDR=10.35.1.0/24
export SUBNET_B_AZ=ap-northeast-1c

# (推奨) EKS public endpoint を許可する CIDR
# - 指定しない場合は RGD の default により "0.0.0.0/0" になります
# - 検証でも可能なら自分のグローバルIP(/32)に絞ってください
# - あえて全開放するなら "0.0.0.0/0" を指定します (非推奨)
export PUBLIC_ACCESS_CIDR="$(curl -fsSL https://checkip.amazonaws.com | tr -d '\n')/32"
```

確認:

```bash
aws sts get-caller-identity
aws configure get region || true
```

## 1) kind 管理クラスタを作る

```bash
kind create cluster --name kany8s-eks --wait 60s
kubectl config use-context kind-kany8s-eks
kubectl get nodes -o wide
```

## 2) (任意) Cluster API core を入れる

`Kany8sControlPlane` 単体でも EKS は作れますが、facade の owner `Cluster` 解決などを実際に踏むには CAPI `Cluster` を作るのが便利です。

注: `docs/eks/manifests/cluster.yaml.tpl` は `cluster.x-k8s.io/v1beta2` の `Cluster` を使います。
管理クラスタに v1beta2 CRD が無い場合は apply が失敗するので、その場合は `docs/eks/manifests/controlplane-only.yaml.tpl` を使ってください。

また、Kany8s は validating webhook を使うため、管理クラスタに cert-manager が必要です。
`clusterctl init` を実行する場合は cert-manager も自動でインストールされます。

```bash
clusterctl version

# v1beta2 contract に合わせて、Core provider の version を明示するのがおすすめです。
# ClusterClass/Topology を使う場合は有効化しておく。
export CLUSTER_TOPOLOGY=true
export CAPI_VERSION=v1.12.2
clusterctl init \
  --core cluster-api:${CAPI_VERSION} \
  --bootstrap kubeadm:${CAPI_VERSION} \
  --control-plane kubeadm:${CAPI_VERSION} \
  --infrastructure docker:${CAPI_VERSION} \
  --wait-providers
kubectl get deployments -n capi-system
```

注:

- CAPD/CABPK 等は今回の EKS 作成には不要ですが、core セットアップを簡単にするため入れています。

### Cluster API を入れない場合 (cert-manager のみ入れる)

`Cluster` を apply しない場合でも、Kany8s の webhook のため cert-manager は必要です。

```bash
export CERT_MANAGER_VERSION=v1.16.3

kubectl apply -f "https://github.com/cert-manager/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml"
kubectl -n cert-manager rollout status deploy/cert-manager --timeout=300s
kubectl -n cert-manager rollout status deploy/cert-manager-webhook --timeout=300s
kubectl -n cert-manager rollout status deploy/cert-manager-cainjector --timeout=300s
```

## 3) kro を入れる

この repo の acceptance と同じパターンで入れます。

```bash
export KRO_VERSION=0.7.1

kubectl create namespace kro-system || true
curl -fsSL -o /tmp/kro-core-install-manifests.yaml \
  "https://github.com/kubernetes-sigs/kro/releases/download/v${KRO_VERSION}/kro-core-install-manifests.yaml"
kubectl apply -f /tmp/kro-core-install-manifests.yaml
kubectl -n kro-system rollout status deploy/kro --timeout=180s

# (v0.7.1 workaround) kro controller に広い RBAC を付与
kubectl apply -f test/acceptance_test/manifests/kro/rbac-unrestricted.yaml
```

## 4) AWS ACK を入れる (iam + ec2 + eks)

ACK の Helm chart は OCI で配布されています。

```bash
export ACK_SYSTEM_NAMESPACE=ack-system
kubectl create namespace "$ACK_SYSTEM_NAMESPACE" || true

# (推奨) ACK controller が読む shared credentials file を Secret として用意
# - `~/.aws/credentials` が無い場合は、環境変数から一時ファイルを作って Secret に入れます。
tmp_creds=""
if [[ -f "$HOME/.aws/credentials" ]]; then
  tmp_creds="$HOME/.aws/credentials"
else
  if [[ -z "${AWS_ACCESS_KEY_ID:-}" || -z "${AWS_SECRET_ACCESS_KEY:-}" ]]; then
    echo "ERROR: ~/.aws/credentials が無く、AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY も未設定です" 1>&2
    exit 1
  fi
  tmp_creds="$(mktemp)"
  cat >"${tmp_creds}" <<EOF
[default]
aws_access_key_id = ${AWS_ACCESS_KEY_ID}
aws_secret_access_key = ${AWS_SECRET_ACCESS_KEY}
EOF
  if [[ -n "${AWS_SESSION_TOKEN:-}" ]]; then
    echo "aws_session_token = ${AWS_SESSION_TOKEN}" >>"${tmp_creds}"
  fi
fi

kubectl -n "$ACK_SYSTEM_NAMESPACE" create secret generic aws-creds \
  --from-file=credentials="${tmp_creds}" \
  --dry-run=client -o yaml | kubectl apply -f -

if [[ "${tmp_creds}" == /tmp/* ]]; then
  rm -f "${tmp_creds}"
fi

aws ecr-public get-login-password --region us-east-1 | \
  helm registry login --username AWS --password-stdin public.ecr.aws

# 各 service controller の最新バージョンを取る (jq が必要)
export IAM_RELEASE_VERSION="$(curl -sL https://api.github.com/repos/aws-controllers-k8s/iam-controller/releases/latest | jq -r '.tag_name | ltrimstr("v")')"
export EC2_RELEASE_VERSION="$(curl -sL https://api.github.com/repos/aws-controllers-k8s/ec2-controller/releases/latest | jq -r '.tag_name | ltrimstr("v")')"
export EKS_RELEASE_VERSION="$(curl -sL https://api.github.com/repos/aws-controllers-k8s/eks-controller/releases/latest | jq -r '.tag_name | ltrimstr("v")')"

helm upgrade --install --create-namespace -n "$ACK_SYSTEM_NAMESPACE" ack-iam-controller \
  oci://public.ecr.aws/aws-controllers-k8s/iam-chart \
  --version="$IAM_RELEASE_VERSION" \
  --set=aws.region="$AWS_REGION" \
  --set=aws.credentials.secretName=aws-creds

helm upgrade --install --create-namespace -n "$ACK_SYSTEM_NAMESPACE" ack-ec2-controller \
  oci://public.ecr.aws/aws-controllers-k8s/ec2-chart \
  --version="$EC2_RELEASE_VERSION" \
  --set=aws.region="$AWS_REGION" \
  --set=aws.credentials.secretName=aws-creds

helm upgrade --install --create-namespace -n "$ACK_SYSTEM_NAMESPACE" ack-eks-controller \
  oci://public.ecr.aws/aws-controllers-k8s/eks-chart \
  --version="$EKS_RELEASE_VERSION" \
  --set=aws.region="$AWS_REGION" \
  --set=aws.credentials.secretName=aws-creds

kubectl -n "$ACK_SYSTEM_NAMESPACE" rollout status deploy/ack-iam-controller-iam-chart --timeout=180s
kubectl -n "$ACK_SYSTEM_NAMESPACE" rollout status deploy/ack-ec2-controller-ec2-chart --timeout=180s
kubectl -n "$ACK_SYSTEM_NAMESPACE" rollout status deploy/ack-eks-controller-eks-chart --timeout=180s
```

## 5) ACK に AWS credentials を渡す

kind 上では IRSA が使えないので、ACK Pod に credentials を渡します。

### A. (推奨) shared credentials file (`~/.aws/credentials`) を使う

Step 4 の `aws-creds` Secret + `--set aws.credentials.secretName=aws-creds` を使っていれば OK です。

### B. (簡単) access key を env でセット

```bash
kubectl -n "$ACK_SYSTEM_NAMESPACE" set env deploy/ack-iam-controller-iam-chart \
  AWS_ACCESS_KEY_ID="$AWS_ACCESS_KEY_ID" \
  AWS_SECRET_ACCESS_KEY="$AWS_SECRET_ACCESS_KEY" \
  AWS_SESSION_TOKEN="${AWS_SESSION_TOKEN:-}" || true

kubectl -n "$ACK_SYSTEM_NAMESPACE" set env deploy/ack-ec2-controller-ec2-chart \
  AWS_ACCESS_KEY_ID="$AWS_ACCESS_KEY_ID" \
  AWS_SECRET_ACCESS_KEY="$AWS_SECRET_ACCESS_KEY" \
  AWS_SESSION_TOKEN="${AWS_SESSION_TOKEN:-}" || true

kubectl -n "$ACK_SYSTEM_NAMESPACE" set env deploy/ack-eks-controller-eks-chart \
  AWS_ACCESS_KEY_ID="$AWS_ACCESS_KEY_ID" \
  AWS_SECRET_ACCESS_KEY="$AWS_SECRET_ACCESS_KEY" \
  AWS_SESSION_TOKEN="${AWS_SESSION_TOKEN:-}" || true
```

## 6) Kany8s を kind に deploy

repo root で実行します。

```bash
export IMG=example.com/kany8s:eks-smoke

make install
make docker-build IMG="$IMG"
kind load docker-image "$IMG" --name kany8s-eks
make deploy IMG="$IMG"

kubectl -n kany8s-system rollout status deployment/kany8s-controller-manager --timeout=180s
```

## 7) EKS ControlPlane RGD を apply

このディレクトリの RGD を使います (VPC/Subnet + IAM Role + EKS Cluster を作成)。

```bash
kubectl apply -f docs/eks/manifests/eks-control-plane-smoke-rgd.yaml
kubectl wait --for=condition=ResourceGraphAccepted --timeout=120s rgd/eks-control-plane-smoke.kro.run
```

## 8) Cluster / Kany8sControlPlane を apply

`PUBLIC_ACCESS_CIDR` を `publicAccessCIDRs` としてテンプレに注入します。

### CAPI `Cluster` も作る場合 (推奨)

```bash
rendered=/tmp/eks-cluster.yaml
sed \
  -e "s|__CLUSTER_NAME__|${CLUSTER_NAME}|g" \
  -e "s|__NAMESPACE__|${NAMESPACE}|g" \
  -e "s|__KUBERNETES_VERSION__|${KUBERNETES_VERSION}|g" \
  -e "s|__AWS_REGION__|${AWS_REGION}|g" \
  -e "s|__VPC_CIDR__|${VPC_CIDR}|g" \
  -e "s|__SUBNET_A_CIDR__|${SUBNET_A_CIDR}|g" \
  -e "s|__SUBNET_A_AZ__|${SUBNET_A_AZ}|g" \
  -e "s|__SUBNET_B_CIDR__|${SUBNET_B_CIDR}|g" \
  -e "s|__SUBNET_B_AZ__|${SUBNET_B_AZ}|g" \
  docs/eks/manifests/cluster.yaml.tpl \
| sed \
  -e 's|^    # publicAccessCIDRs:|    publicAccessCIDRs:|' \
  -e "s|^    #   - \"203.0.113.10/32\"|      - \"${PUBLIC_ACCESS_CIDR}\"|" \
>"${rendered}"

kubectl apply -f "${rendered}"
```

### `Kany8sControlPlane` 単体で動かす場合

```bash
rendered=/tmp/eks-controlplane.yaml
sed \
  -e "s|__CLUSTER_NAME__|${CLUSTER_NAME}|g" \
  -e "s|__NAMESPACE__|${NAMESPACE}|g" \
  -e "s|__KUBERNETES_VERSION__|${KUBERNETES_VERSION}|g" \
  -e "s|__AWS_REGION__|${AWS_REGION}|g" \
  -e "s|__VPC_CIDR__|${VPC_CIDR}|g" \
  -e "s|__SUBNET_A_CIDR__|${SUBNET_A_CIDR}|g" \
  -e "s|__SUBNET_A_AZ__|${SUBNET_A_AZ}|g" \
  -e "s|__SUBNET_B_CIDR__|${SUBNET_B_CIDR}|g" \
  -e "s|__SUBNET_B_AZ__|${SUBNET_B_AZ}|g" \
  docs/eks/manifests/controlplane-only.yaml.tpl \
| sed \
  -e 's|^    # publicAccessCIDRs:|    publicAccessCIDRs:|' \
  -e "s|^    #   - \"203.0.113.10/32\"|      - \"${PUBLIC_ACCESS_CIDR}\"|" \
>"${rendered}"

kubectl apply -f "${rendered}"
```

## 9) 進捗を見る

```bash
# Kany8s facade が Ready になるまで待つ (EKS 作成は 10-20 分かかることがあります)
kubectl -n "$NAMESPACE" wait --for=condition=Ready --timeout=25m kany8scontrolplane/"$CLUSTER_NAME" || true

kubectl -n "$NAMESPACE" get kany8scontrolplane "$CLUSTER_NAME" -o wide

# ACK (EC2)
kubectl -n "$NAMESPACE" get vpcs.ec2.services.k8s.aws "${CLUSTER_NAME}-vpc" -o wide || true
kubectl -n "$NAMESPACE" get subnets.ec2.services.k8s.aws "${CLUSTER_NAME}-subnet-a" -o wide || true
kubectl -n "$NAMESPACE" get subnets.ec2.services.k8s.aws "${CLUSTER_NAME}-subnet-b" -o wide || true

# ACK (EKS)
kubectl -n "$NAMESPACE" get clusters.eks.services.k8s.aws "$CLUSTER_NAME" -o wide
kubectl -n "$NAMESPACE" describe clusters.eks.services.k8s.aws "$CLUSTER_NAME" || true

# ACK (IAM Role)
kubectl -n "$NAMESPACE" get roles.iam.services.k8s.aws "${CLUSTER_NAME}-eks-control-plane" -o wide

# kro instance (CRD 名は RGD の schema.kind から生成される)
kubectl api-resources --api-group=kro.run | grep -E 'awsbyonetwork|ekscontrolplane' || true
kubectl -n "$NAMESPACE" get ekscontrolplanes.kro.run "$CLUSTER_NAME" -o yaml || true
kubectl -n "$NAMESPACE" get ekscontrolplanebyos.kro.run "$CLUSTER_NAME" -o yaml || true
```

AWS 側:

```bash
# NOTE: apply 直後は AWS 側に反映されるまで少し時間がかかり、ResourceNotFound になることがあります。
aws eks describe-cluster --region "$AWS_REGION" --name "$CLUSTER_NAME" \
  --query 'cluster.[status,endpoint,version]' --output table
```

## 10) Cleanup

詳しい手順は `docs/eks/cleanup.md` を参照してください。

1) Kubernetes 側を消す (ACK finalizer が AWS リソース削除を走らせる)

```bash
kubectl -n "$NAMESPACE" delete cluster.cluster.x-k8s.io "$CLUSTER_NAME" --ignore-not-found
kubectl -n "$NAMESPACE" delete kany8scontrolplane "$CLUSTER_NAME" --ignore-not-found
```

2) ACK リソースが消えたことを確認

```bash
kubectl -n "$NAMESPACE" get vpcs.ec2.services.k8s.aws "${CLUSTER_NAME}-vpc" -o name || true
kubectl -n "$NAMESPACE" get subnets.ec2.services.k8s.aws "${CLUSTER_NAME}-subnet-a" -o name || true
kubectl -n "$NAMESPACE" get subnets.ec2.services.k8s.aws "${CLUSTER_NAME}-subnet-b" -o name || true

kubectl -n "$NAMESPACE" get clusters.eks.services.k8s.aws "$CLUSTER_NAME" -o name || true
kubectl -n "$NAMESPACE" get roles.iam.services.k8s.aws "${CLUSTER_NAME}-eks-control-plane" -o name || true

aws eks describe-cluster --region "$AWS_REGION" --name "$CLUSTER_NAME" || true
```

3) 最後に kind を消す

```bash
kind delete cluster --name kany8s-eks
```

---

## トラブルシュート (よくある)

- RGD apply 時に `ResourceGraphAccepted=False`:
  - ACK の CRD がまだ入っていない (eks/iam/ec2 controller install 前に RGD を apply した)
  - kro v0.7.1 の static analysis で落ちている (describe で詳細を見る)
    - `kubectl describe rgd eks-control-plane-smoke.kro.run`

- ACK の EKS Cluster が `terminal` っぽい状態で進まない/イベントに AWS エラー:
  - `kubectl -n "$ACK_SYSTEM_NAMESPACE" logs deploy/ack-eks-controller-eks-chart --tail=200`
  - role policy が足りない / subnets が不正 / region 違い など

- Cleanup で AWS 側にリソースが残る:
  - 管理クラスタ(kind)を先に消してしまった場合、ACK 経由の削除ができません。
  - AWS CLI/Console で EKS Cluster / IAM Role / VPC / Subnet の削除を実行してください。
