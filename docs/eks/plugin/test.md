# Test: `eks-kubeconfig-rotator`

このドキュメントは `eks-kubeconfig-rotator` を kind 管理クラスタ上で動かし、
EKS BYO network サンプル（ClusterClass/Topology）に対して

- `<cluster>-kubeconfig`（token 埋め込み）が生成/ローテーションされる
- CAPI `RemoteConnectionProbe=True`（ひいては `Cluster Available=True`）に到達できる

ことを確認する手順です。

## 前提

- kind 管理クラスタ + CAPI core + kro + ACK(iam/ec2/eks) + Kany8s がセットアップ済み
  - セットアップ手順: `docs/eks/README.md`
- BYO network 用の manifest を apply できる状態
  - サンプル: `docs/eks/byo-network/README.md`

注:

- management cluster から EKS endpoint へ到達できない構成（private endpoint 等）では `RemoteConnectionProbe` は成立しません。
- この plugin は ACK(EKS) の `status.endpoint` / `status.certificateAuthority.data` を入力に使います。

## 1) ローカルテスト（unit）

```bash
make test
```

## 2) plugin のビルド/デプロイ（kind）

kind クラスタ名は `docs/eks/README.md` と同じく `kany8s-eks` を前提にします。

```bash
export EKS_PLUGIN_IMG=example.com/eks-kubeconfig-rotator:dev

make docker-build-eks-plugin EKS_PLUGIN_IMG="$EKS_PLUGIN_IMG"
kind load docker-image "$EKS_PLUGIN_IMG" --name kany8s-eks

make deploy-eks-plugin EKS_PLUGIN_IMG="$EKS_PLUGIN_IMG"
kubectl -n ack-system rollout status deploy/eks-kubeconfig-rotator --timeout=180s
```

ログ確認:

```bash
kubectl -n ack-system logs deploy/eks-kubeconfig-rotator -c manager -f
```

## 3) BYO サンプルで EKS ControlPlane を作る

BYO サンプルの apply は `docs/eks/byo-network/README.md` / `docs/eks/README.md` の BYO セクションに従ってください。

ここでは最短の確認ポイントだけ列挙します。

### 3.1 RGD / ClusterClass の apply

```bash
kubectl apply -f docs/eks/byo-network/manifests/aws-byo-network-rgd.yaml
kubectl wait --for=condition=ResourceGraphAccepted --timeout=120s rgd/aws-byo-network.kro.run

kubectl apply -f docs/eks/byo-network/manifests/eks-control-plane-byo-rgd.yaml
kubectl wait --for=condition=ResourceGraphAccepted --timeout=120s rgd/eks-control-plane-byo.kro.run

kubectl -n "$NAMESPACE" apply -f docs/eks/byo-network/manifests/clusterclass-eks-byo.yaml
```

### 3.2 Topology Cluster を apply

```bash
: "${EKS_ACCESS_MODE:=API_AND_CONFIG_MAP}"
: "${EKS_ENDPOINT_PRIVATE_ACCESS:=true}"
: "${EKS_ENDPOINT_PUBLIC_ACCESS:=true}"

rendered=/tmp/eks-cluster-byo.yaml
sed \
  -e "s|__CLUSTER_NAME__|${CLUSTER_NAME}|g" \
  -e "s|__NAMESPACE__|${NAMESPACE}|g" \
  -e "s|__KUBERNETES_VERSION__|${KUBERNETES_VERSION}|g" \
  -e "s|__AWS_REGION__|${AWS_REGION}|g" \
  -e "s|__EKS_VERSION__|${EKS_VERSION}|g" \
  -e "s|__CONTROL_PLANE_SUBNET_ID_1__|${CONTROL_PLANE_SUBNET_ID_1}|g" \
  -e "s|__CONTROL_PLANE_SUBNET_ID_2__|${CONTROL_PLANE_SUBNET_ID_2}|g" \
  -e "s|__NODE_SUBNET_ID_1__|${NODE_SUBNET_ID_1}|g" \
  -e "s|__NODE_SUBNET_ID_2__|${NODE_SUBNET_ID_2}|g" \
  -e "s|__SECURITY_GROUP_IDS_JSON__|${SECURITY_GROUP_IDS_JSON}|g" \
  -e "s|__PUBLIC_ACCESS_CIDR__|${PUBLIC_ACCESS_CIDR}|g" \
  -e "s|__EKS_ACCESS_MODE__|${EKS_ACCESS_MODE}|g" \
  -e "s|__EKS_ENDPOINT_PRIVATE_ACCESS__|${EKS_ENDPOINT_PRIVATE_ACCESS}|g" \
  -e "s|__EKS_ENDPOINT_PUBLIC_ACCESS__|${EKS_ENDPOINT_PUBLIC_ACCESS}|g" \
  docs/eks/byo-network/manifests/cluster.yaml.tpl > "${rendered}"

kubectl apply -f "${rendered}"
```

## 4) plugin を有効化（opt-in）

plugin はデフォルト無効です。
CAPI `Cluster` に annotation を付与して有効化します。

```bash
kubectl -n "$NAMESPACE" annotate cluster "$CLUSTER_NAME" \
  eks.kany8s.io/kubeconfig-rotator=enabled --overwrite
```

## 5) 期待するリソースと確認

### 5.1 ACK EKS Cluster 名（name ズレの確認）

Topology の場合、ACK の EKS `Cluster` は `Cluster.metadata.name` ではなく
`Cluster.spec.controlPlaneRef.name`（Kany8sControlPlane 名）で作られることがあります。

```bash
CP_NAME="$(kubectl -n "$NAMESPACE" get cluster "$CLUSTER_NAME" -o jsonpath='{.spec.controlPlaneRef.name}')"
echo "controlPlaneRef.name=${CP_NAME}"

kubectl -n "$NAMESPACE" get clusters.eks.services.k8s.aws "$CP_NAME"
```

### 5.2 kubeconfig Secret が生成される

```bash
kubectl -n "$NAMESPACE" get secret "${CLUSTER_NAME}-kubeconfig"
kubectl -n "$NAMESPACE" get secret "${CLUSTER_NAME}-kubeconfig-exec"
```

probe 用 Secret（token 埋め込み）のメタデータ確認:

```bash
kubectl -n "$NAMESPACE" get secret "${CLUSTER_NAME}-kubeconfig" \
  -o jsonpath='{.type}{"\n"}{.metadata.labels.cluster\.x-k8s\.io/cluster-name}{"\n"}{.metadata.annotations.eks\.kany8s\.io/managed-by}{"\n"}{.metadata.annotations.eks\.kany8s\.io/cluster-name}{"\n"}{.metadata.annotations.eks\.kany8s\.io/token-expiration-rfc3339}{"\n"}'
```

### 5.3 kubectl で接続できる（人間用 exec kubeconfig）

```bash
kubectl -n "$NAMESPACE" get secret "${CLUSTER_NAME}-kubeconfig-exec" -o jsonpath='{.data.value}' | base64 -d \
  > "/tmp/${CLUSTER_NAME}-kubeconfig"

KUBECONFIG="/tmp/${CLUSTER_NAME}-kubeconfig" kubectl get namespaces
```

### 5.4 CAPI `RemoteConnectionProbe=True` / `Available=True` を確認

```bash
kubectl -n "$NAMESPACE" wait --for=condition=RemoteConnectionProbe=True cluster "$CLUSTER_NAME" --timeout=20m
kubectl -n "$NAMESPACE" wait --for=condition=Available=True cluster "$CLUSTER_NAME" --timeout=20m

kubectl -n "$NAMESPACE" get cluster "$CLUSTER_NAME" -o jsonpath='{range .status.conditions[*]}{.type}={.status} {.reason}{"\n"}{end}'
```

## 6) (任意) token ローテーションの確認

`eks.kany8s.io/token-expiration-rfc3339` が更新され続け、かつ `RemoteConnectionProbe=True` が維持されることを確認します。

```bash
kubectl -n "$NAMESPACE" get secret "${CLUSTER_NAME}-kubeconfig" \
  -o jsonpath='{.metadata.annotations.eks\.kany8s\.io/token-expiration-rfc3339}{"\n"}'

sleep 600

kubectl -n "$NAMESPACE" get secret "${CLUSTER_NAME}-kubeconfig" \
  -o jsonpath='{.metadata.annotations.eks\.kany8s\.io/token-expiration-rfc3339}{"\n"}'

kubectl -n "$NAMESPACE" wait --for=condition=RemoteConnectionProbe=True cluster "$CLUSTER_NAME" --timeout=2m
```

## トラブルシュート

- Secret が作られない
  - `kubectl -n ack-system logs deploy/eks-kubeconfig-rotator -c manager -f`
  - `kubectl -n "$NAMESPACE" get clusters.eks.services.k8s.aws`（ACK EKS Cluster の存在/namespace）
  - `kubectl -n "$NAMESPACE" get cluster "$CLUSTER_NAME" -o yaml`（annotation / controlPlaneRef）

- `RemoteConnectionProbe=False`
  - EKS endpoint の Public access CIDR に、management cluster の egress IP が含まれているか
  - `kubectl -n "$NAMESPACE" get secret "${CLUSTER_NAME}-kubeconfig" -o yaml`（kubeconfig の server/CA/token）

## Cleanup

```bash
make undeploy-eks-plugin

# EKS リソース削除は `docs/eks/cleanup.md` を参照
```
