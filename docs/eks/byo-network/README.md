# EKS BYO Network Sample (ClusterClass/Topology)

このディレクトリは「既存 VPC/Subnet(BYO network) 上に EKS ControlPlane だけを作る」ためのサンプル一式です。

- 設計: `docs/eks/byo-network/design.md`
- 管理クラスタ(kind) / CAPI / kro / ACK / Kany8s のセットアップ: `docs/eks/README.md`
- 変数の決め方: `docs/eks/values.md`
- 削除: `docs/eks/cleanup.md`

## 含まれるマニフェスト

- (任意) ネットワーク bootstrap (VPC/Subnet を ACK で新規作成): `docs/eks/byo-network/manifests/bootstrap-network.yaml.tpl`
- `docs/eks/byo-network/manifests/aws-byo-network-rgd.yaml`
- `docs/eks/byo-network/manifests/eks-control-plane-byo-rgd.yaml`
- `docs/eks/byo-network/manifests/clusterclass-eks-byo.yaml`
- `docs/eks/byo-network/manifests/cluster.yaml.tpl`

## ネットワークが無い場合（bootstrap; AWS CLI/Console 不要）

BYO では通常「既存 subnet IDs」が必要です。
まだ VPC/Subnet が無い場合は、このテンプレで ACK(EC2) に作成させてから、その subnet IDs を BYO の `Cluster.spec.topology.variables` に渡してください。

```bash
# values は `docs/eks/values.md` を参照
export NETWORK_NAME=demo-byo-network

rendered=/tmp/eks-byo-bootstrap-network.yaml
sed \
  -e "s|__NETWORK_NAME__|${NETWORK_NAME}|g" \
  -e "s|__NAMESPACE__|${NAMESPACE}|g" \
  -e "s|__AWS_REGION__|${AWS_REGION}|g" \
  -e "s|__VPC_CIDR__|${VPC_CIDR}|g" \
  -e "s|__SUBNET_A_CIDR__|${SUBNET_A_CIDR}|g" \
  -e "s|__SUBNET_A_AZ__|${SUBNET_A_AZ}|g" \
  -e "s|__SUBNET_B_CIDR__|${SUBNET_B_CIDR}|g" \
  -e "s|__SUBNET_B_AZ__|${SUBNET_B_AZ}|g" \
  docs/eks/byo-network/manifests/bootstrap-network.yaml.tpl > "${rendered}"

kubectl apply -f "${rendered}"

# subnet IDs を取得 (ACK が反映するまで少し待つことがあります)
export SUBNET_ID_1="$(kubectl -n "$NAMESPACE" get subnets.ec2.services.k8s.aws "${NETWORK_NAME}-subnet-a" -o jsonpath='{.status.subnetID}')"
export SUBNET_ID_2="$(kubectl -n "$NAMESPACE" get subnets.ec2.services.k8s.aws "${NETWORK_NAME}-subnet-b" -o jsonpath='{.status.subnetID}')"

echo "SUBNET_ID_1=${SUBNET_ID_1}"
echo "SUBNET_ID_2=${SUBNET_ID_2}"
```

## 最短 apply（セットアップ完了後）

```bash
kubectl apply -f docs/eks/byo-network/manifests/aws-byo-network-rgd.yaml
kubectl wait --for=condition=ResourceGraphAccepted --timeout=120s rgd/aws-byo-network.kro.run

kubectl apply -f docs/eks/byo-network/manifests/eks-control-plane-byo-rgd.yaml
kubectl wait --for=condition=ResourceGraphAccepted --timeout=120s rgd/eks-control-plane-byo.kro.run

# ClusterClass + Template は Cluster と同じ namespace へ apply する
kubectl -n "$NAMESPACE" apply -f docs/eks/byo-network/manifests/clusterclass-eks-byo.yaml
```

Topology Cluster の render/apply は `docs/eks/README.md` の BYO セクションを参照してください。
