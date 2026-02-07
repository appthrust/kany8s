# Cleanup (EKS smoke / BYO network)

このドキュメントは `docs/eks/README.md` の手順で作成した EKS (Control Plane) + 付随リソースを削除する手順です。

BYO network の削除セマンティクス:

- 削除対象: EKS Cluster / IAM Role（ACK 管理）
- 削除対象外: 既存 VPC/Subnet（BYO infra RGD は ConfigMap のみを管理）

重要:

- kind 管理クラスタを先に消すと、ACK の finalizer による削除ができず AWS リソースが残ることがあります。
- 必ず「Kubernetes 側のリソース削除」→「AWS 側リソースが消えたことを確認」→「最後に kind を削除」の順で実施してください。

## 0) 対象の確認

```bash
export AWS_REGION=ap-northeast-1
export NAMESPACE=default

# 対象の一覧 (この中からクラスタ名を決める)
kubectl -n "$NAMESPACE" get kany8scontrolplane -o wide
kubectl -n "$NAMESPACE" get clusters.eks.services.k8s.aws -o wide || true
kubectl -n "$NAMESPACE" get ekscontrolplanes.kro.run -o wide || true
kubectl -n "$NAMESPACE" get ekscontrolplanebyos.kro.run -o wide || true
kubectl api-resources --api-group=kro.run | grep -E 'ekscontrolplane|awsbyonetwork' || true
```

削除するクラスタ名をセット:

```bash
export CLUSTER_NAME=<your-cluster-name>
```

## 1) Kubernetes 側の削除 (ACK/kro に削除を走らせる)

`Kany8sControlPlane` -> kro instance は 1:1 で作られるため、
ControlPlane を消すと GC により kro instance も消え、そこから ACK リソース削除が走ります。

```bash
# もし CAPI Cluster を apply しているなら先に消す
kubectl -n "$NAMESPACE" delete cluster.cluster.x-k8s.io "$CLUSTER_NAME" --ignore-not-found

# Kany8s facade
kubectl -n "$NAMESPACE" delete kany8scontrolplane "$CLUSTER_NAME" --ignore-not-found
kubectl -n "$NAMESPACE" delete kany8scluster "$CLUSTER_NAME" --ignore-not-found

# kro instance (残っていれば明示的に削除)
kubectl -n "$NAMESPACE" delete ekscontrolplanes.kro.run "$CLUSTER_NAME" --ignore-not-found || true
kubectl -n "$NAMESPACE" delete ekscontrolplanebyos.kro.run "$CLUSTER_NAME" --ignore-not-found || true
kubectl -n "$NAMESPACE" delete awsbyonetworks.kro.run "$CLUSTER_NAME" --ignore-not-found || true
```

## 2) AWS リソースが消えるまで待つ

EKS cluster の削除は 10-20 分かかることがあります。

```bash
# ACK (EKS) が消えるまで待つ
kubectl -n "$NAMESPACE" wait --for=delete --timeout=40m clusters.eks.services.k8s.aws/"$CLUSTER_NAME" || true

# ACK (EC2): smoke フローのみ対象
kubectl -n "$NAMESPACE" wait --for=delete --timeout=20m subnets.ec2.services.k8s.aws/"${CLUSTER_NAME}-subnet-a" || true
kubectl -n "$NAMESPACE" wait --for=delete --timeout=20m subnets.ec2.services.k8s.aws/"${CLUSTER_NAME}-subnet-b" || true
kubectl -n "$NAMESPACE" wait --for=delete --timeout=20m vpcs.ec2.services.k8s.aws/"${CLUSTER_NAME}-vpc" || true

# ACK (IAM)
kubectl -n "$NAMESPACE" wait --for=delete --timeout=10m roles.iam.services.k8s.aws/"${CLUSTER_NAME}-eks-control-plane" || true
```

AWS CLI でも確認:

```bash
aws eks describe-cluster --region "$AWS_REGION" --name "$CLUSTER_NAME" || true
```

BYO 補足:

- `kubectl delete cluster ...` 後も、既存 VPC/Subnet はそのまま残るのが正しい挙動です。
- BYO フローで VPC/Subnet が削除される場合は、BYO ではない manifest（`*-smoke-*`）を適用していないか確認してください。

## 3) (任意) 管理クラスタ(kind)も消す

```bash
kind delete cluster --name kany8s-eks
```

## トラブルシュート

- `kubectl wait --for=delete` がタイムアウトする
  - ACK controller logs:
    - `kubectl -n ack-system logs deploy/ack-eks-controller-eks-chart --tail=200`
    - `kubectl -n ack-system logs deploy/ack-ec2-controller-ec2-chart --tail=200`
    - `kubectl -n ack-system logs deploy/ack-iam-controller-iam-chart --tail=200`
  - ACK resource の conditions/event を確認:
    - `kubectl -n "$NAMESPACE" describe clusters.eks.services.k8s.aws "$CLUSTER_NAME"`

- kind を先に消して AWS に残った
  - AWS CLI/Console で削除してください。
  - 目安:
    - `aws eks delete-cluster --region "$AWS_REGION" --name "$CLUSTER_NAME"`
