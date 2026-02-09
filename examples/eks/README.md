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

2) (任意) ネットワークが無い場合は作成

- `examples/eks/manifests/bootstrap-network-private-nat.yaml.tpl` を render して apply
- できた private subnet IDs を控える

3) RGD/ClusterClass を apply

```bash
kubectl -n default apply -k examples/eks/manifests/

kubectl wait --for=condition=ResourceGraphAccepted --timeout=120s rgd/aws-byo-network.kro.run
kubectl wait --for=condition=ResourceGraphAccepted --timeout=120s rgd/eks-control-plane-byo.kro.run
```

4) Cluster を作成

- `examples/eks/manifests/cluster.yaml.tpl` を render して apply
  - このテンプレは `eks.kany8s.io/kubeconfig-rotator=enabled` / `eks.kany8s.io/karpenter=enabled` を最初から付けます
  - `vpc-security-group-ids=[]` を既定にしているため、bootstrapper が node SG を自動作成します

5) plugins をデプロイ

```bash
kubectl apply -k examples/eks/management/
```

NOTE:

- plugin image は既定で `example.com/*:latest` を参照します。
  - kind に入れる場合は、同じ tag で build + `kind load docker-image` するか、kustomize で image を差し替えてください。
  - 既存の Make targets でも可: `make docker-build-eks-plugin` / `make deploy-eks-plugin`、`make docker-build-eks-karpenter-bootstrapper` / `make deploy-eks-karpenter-bootstrapper`

6) 需要を作って node join を確認

- workload kubeconfig を取り出して `examples/eks/manifests/karpenter-smoke.yaml` を apply

削除/リセット:

- `hack/eks-fargate-dev-reset.sh` (CAPI Cluster delete + (任意) network delete)
