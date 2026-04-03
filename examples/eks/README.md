# examples/eks

EKS (BYO private subnets) + Fargate bootstrap + Karpenter の "標準" 構成をまとめた例です。

この例で使うもの:

- management cluster: kind (ACK iam/ec2/eks + kro + Flux + plugins)
- workload cluster: EKS (Fargate + Karpenter)

マニフェスト一覧:

- (任意) private subnets + NAT を ACK(EC2) で作る: `examples/eks/manifests/bootstrap-network-private-nat.yaml.tpl`
- BYO network の入力チェック用 RGD: `examples/eks/manifests/aws-byo-network-rgd.yaml`
- BYO EKS control plane RGD: `examples/eks/manifests/eks-control-plane-byo-rgd.yaml`
- ClusterClass/Template: `examples/eks/manifests/clusterclass-eks-byo.yaml`
- Topology Cluster テンプレ: `examples/eks/manifests/cluster.yaml.tpl`
- 需要作成用 workload: `examples/eks/manifests/karpenter-smoke.yaml`

この例が前提にする plugins:

- kubeconfig rotator: `kubectl apply -k examples/eks/management/eks-kubeconfig-rotator/`
- karpenter bootstrapper: `kubectl apply -k examples/eks/management/eks-karpenter-bootstrapper/`

使い方 (概要):

1) management(kind) のセットアップ

- `docs/eks/README.md` に従って kro + ACK(iam/ec2/eks) + Flux を導入

## コマンド一覧 (コピペ用)

この節は「この examples で EKS + Fargate + Karpenter を立ち上げる時に打つコマンド」を列挙します。
前提が揃っていれば、この順でそのまま動きます。

0) 共通

```bash
export NAMESPACE=default
export AWS_REGION=ap-northeast-1
```

1) (任意) ネットワークが無い場合は作成

- `examples/eks/manifests/bootstrap-network-private-nat.yaml.tpl` を render して apply
- できた private subnet IDs を控える

```bash
export NETWORK_NAME=demo-eks-fargate-net

export VPC_CIDR=10.36.0.0/16

export PUBLIC_SUBNET_A_CIDR=10.36.2.0/24
export PUBLIC_SUBNET_A_AZ=ap-northeast-1a

export PRIVATE_SUBNET_A_CIDR=10.36.0.0/24
export PRIVATE_SUBNET_A_AZ=ap-northeast-1a

export PRIVATE_SUBNET_B_CIDR=10.36.1.0/24
export PRIVATE_SUBNET_B_AZ=ap-northeast-1c

rendered=/tmp/eks-bootstrap-network-private-nat.yaml
sed \
  -e "s|__NETWORK_NAME__|${NETWORK_NAME}|g" \
  -e "s|__NAMESPACE__|${NAMESPACE}|g" \
  -e "s|__AWS_REGION__|${AWS_REGION}|g" \
  -e "s|__VPC_CIDR__|${VPC_CIDR}|g" \
  -e "s|__PUBLIC_SUBNET_A_CIDR__|${PUBLIC_SUBNET_A_CIDR}|g" \
  -e "s|__PUBLIC_SUBNET_A_AZ__|${PUBLIC_SUBNET_A_AZ}|g" \
  -e "s|__PRIVATE_SUBNET_A_CIDR__|${PRIVATE_SUBNET_A_CIDR}|g" \
  -e "s|__PRIVATE_SUBNET_A_AZ__|${PRIVATE_SUBNET_A_AZ}|g" \
  -e "s|__PRIVATE_SUBNET_B_CIDR__|${PRIVATE_SUBNET_B_CIDR}|g" \
  -e "s|__PRIVATE_SUBNET_B_AZ__|${PRIVATE_SUBNET_B_AZ}|g" \
  examples/eks/manifests/bootstrap-network-private-nat.yaml.tpl > "${rendered}"

kubectl apply -f "${rendered}"

# private subnet IDs を取得 (ACK が反映するまで少し待つことがあります)
export SUBNET_ID_1="$(kubectl -n "$NAMESPACE" get subnets.ec2.services.k8s.aws "${NETWORK_NAME}-subnet-private-a" -o jsonpath='{.status.subnetID}')"
export SUBNET_ID_2="$(kubectl -n "$NAMESPACE" get subnets.ec2.services.k8s.aws "${NETWORK_NAME}-subnet-private-b" -o jsonpath='{.status.subnetID}')"

echo "SUBNET_ID_1=${SUBNET_ID_1}"
echo "SUBNET_ID_2=${SUBNET_ID_2}"
```

既に subnet IDs が分かっている場合は、このステップを skip して次を設定してください:

```bash
export SUBNET_ID_1=subnet-...
export SUBNET_ID_2=subnet-...
```

2) RGD/ClusterClass を apply

```bash
kubectl -n "$NAMESPACE" apply -k examples/eks/manifests/

kubectl wait --for=condition=ResourceGraphAccepted --timeout=120s rgd/aws-byo-network.kro.run
kubectl wait --for=condition=ResourceGraphAccepted --timeout=120s rgd/eks-control-plane-byo.kro.run
```

3) Cluster を作成

- `examples/eks/manifests/cluster.yaml.tpl` を render して apply
  - このテンプレは `eks.kany8s.io/kubeconfig-rotator=enabled` / `eks.kany8s.io/karpenter=enabled` を最初から付けます
- `vpc-security-group-ids=[]` を既定にしているため、bootstrapper が node SG を自動作成し、`vpc-node-security-group-ids` に注入します
  - さらに `vpc-security-group-ids` も空の場合、同じ SG を `vpc-security-group-ids` にも注入します（EKS 側の `VpcConfigUpdate` を誘発し得るため、直後の削除で "update in progress" に当たることがあります）

```bash
export CLUSTER_NAME="demo-eks-fargate-$(date +%Y%m%d%H%M%S)"

# CAPI Topology は semver が必須 (例: v1.35.0)
export KUBERNETES_VERSION=v1.35.0

# EKS 自体は major.minor (例: 1.35)
export EKS_VERSION=1.35

# EKS endpoint (public access) を絞る CIDR
export PUBLIC_ACCESS_CIDR="$(curl -fsSL https://checkip.amazonaws.com | tr -d '\n')/32"

rendered=/tmp/${CLUSTER_NAME}.yaml
sed \
  -e "s|__CLUSTER_NAME__|${CLUSTER_NAME}|g" \
  -e "s|__NAMESPACE__|${NAMESPACE}|g" \
  -e "s|__KUBERNETES_VERSION__|${KUBERNETES_VERSION}|g" \
  -e "s|__AWS_REGION__|${AWS_REGION}|g" \
  -e "s|__EKS_VERSION__|${EKS_VERSION}|g" \
  -e "s|__SUBNET_ID_1__|${SUBNET_ID_1}|g" \
  -e "s|__SUBNET_ID_2__|${SUBNET_ID_2}|g" \
  -e "s|__PUBLIC_ACCESS_CIDR__|${PUBLIC_ACCESS_CIDR}|g" \
  examples/eks/manifests/cluster.yaml.tpl > "${rendered}"

kubectl apply -f "${rendered}"
```

4) plugins をデプロイ

```bash
# kind (aws-creds mount) の既定オーバーレイ
kubectl apply -k examples/eks/management/

# 実クラスタ (IRSA) で deploy する場合
kubectl apply -k config/overlays/eks-plugin/irsa
kubectl apply -k config/overlays/eks-karpenter-bootstrapper/irsa
```

NOTE:

- plugin image は `example.com/*:<tag>` を参照します（適用される tag は `kustomization.yaml` の `images:` に依存します）。
  - kind に入れる場合は、同じ tag で build + `kind load docker-image` するか、kustomize で image を差し替えてください。
  - 既存の Make targets でも可: `make docker-build-eks-plugin` / `make deploy-eks-plugin`、`make docker-build-eks-karpenter-bootstrapper` / `make deploy-eks-karpenter-bootstrapper`

5) 需要を作って node join を確認

- probe/Controller 用は `<cluster>-kubeconfig`（token 埋め込み）を利用
- 人間の `kubectl` は `<cluster>-kubeconfig-exec`（`aws eks get-token`）を推奨
- ここでは smoke 用に probe kubeconfig を使って `examples/eks/manifests/karpenter-smoke.yaml` を apply

```bash
# probe/Controller 向け kubeconfig（短命 token 埋め込み）
kube=/tmp/${CLUSTER_NAME}.kubeconfig
kubectl -n "$NAMESPACE" get secret "${CLUSTER_NAME}-kubeconfig" -o jsonpath='{.data.value}' | base64 -d > "$kube"
chmod 600 "$kube"

# 人間向け kubeconfig（aws cli の exec 方式）
human_kube=/tmp/${CLUSTER_NAME}.kubeconfig-exec
kubectl -n "$NAMESPACE" get secret "${CLUSTER_NAME}-kubeconfig-exec" -o jsonpath='{.data.value}' | base64 -d > "$human_kube"
chmod 600 "$human_kube"

kubectl --kubeconfig "$kube" get pods -A -o wide
kubectl --kubeconfig "$kube" get nodes -o wide

# 需要を作る
kubectl --kubeconfig "$kube" apply -f examples/eks/manifests/karpenter-smoke.yaml
kubectl --kubeconfig "$kube" -n default get pods -l app=karpenter-smoke -o wide

kubectl --kubeconfig "$kube" get nodeclaim -A -o wide
kubectl --kubeconfig "$kube" get nodes -o wide
```

NOTE:

- `error: You must be logged in to the server (Unauthorized)` が出たら token が切れています。
  - `kubectl -n "$NAMESPACE" get secret "${CLUSTER_NAME}-kubeconfig" ...` をもう一度実行して kubeconfig を更新してください。

削除/リセット:

- `hack/eks-fargate-dev-reset.sh` (CAPI Cluster delete + (任意) network delete)
