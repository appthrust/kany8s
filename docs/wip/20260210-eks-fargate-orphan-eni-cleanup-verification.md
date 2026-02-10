# WIP: orphan ENI cleanup (EKS Fargate + Karpenter) verification (2026-02-10)

## Goal

- `eks-karpenter-bootstrapper` の delete/finalizer で orphan ENI を削除し、node SecurityGroup deletion が `DependencyViolation` で詰まる確率を下げる
- 総合試験として「作成 -> Karpenter で node join -> ダミー orphan ENI を作る -> Cluster delete -> orphan ENI が自動削除される」を確認する

## Environment

- management cluster: kind (`kind-kany8s-eks`)
- namespace: `default`
- region: `ap-northeast-1`
- ClusterClass: `default/kany8s-eks-byo`

## What changed (under test)

- `eks-karpenter-bootstrapper` の Cluster delete フローに orphan ENI cleanup を追加
  - 対象: bootstrapper-managed node SecurityGroup にぶら下がる ENI
  - candidate ENI 条件:
    - `Attachment == null`
    - `Status == available`
    - tag `eks:eni:owner=amazon-vpc-cni`
  - AWS API:
    - `DescribeNetworkInterfaces` -> `DeleteNetworkInterface`
  - Event reason: `OrphanENICleanup`

設計メモ: `docs/eks/fargate/issues/cleanup.md`

## Steps / Progress

### 1) Deploy updated plugins to management cluster

- images:
  - `example.com/eks-kubeconfig-rotator:dev-20260210104558`
  - `example.com/eks-karpenter-bootstrapper:dev-20260210104558`
- kind に load + `make deploy-*` で反映

### 2) Create test Cluster

- CAPI Cluster:
  - `default/demo-eks-eni-cleanup-20260210104812`
- template:
  - `examples/eks/manifests/cluster.yaml.tpl`
- variables:
  - `vpc-security-group-ids: []` (bootstrapper が node SG を ACK で作成し、Topology variable に inject する想定)
  - `vpc-subnet-ids: [subnet-0abf861a64491f95b, subnet-053e7b45c7957a303]`
  - `eks-public-access-cidrs: [<my-ip>/32]`

ACK EKS Cluster:

- `default/demo-eks-eni-cleanup-20260210104812-chj28`

### 3) Issue: ClusterClass drift blocks topology injection

bootstrapper が `vpc-node-security-group-ids` を patch しようとしたが、ClusterClass 側に variable 定義が無く admission webhook に拒否された。

- error (bootstrapper logs):
  - `spec.topology.variables[vpc-node-security-group-ids]: ... variable is not defined`

影響:

- `vpc-node-security-group-ids` が入らず、default NodePool/EC2NodeClass の配布（CRS）が進まず
- workload 側の `karpenter` / `coredns` pod が Pending のまま（node も fargate も来ない）

対応:

```bash
kubectl apply -f examples/eks/manifests/clusterclass-eks-byo.yaml
```

結果:

- `vpc-security-group-ids` と `vpc-node-security-group-ids` の両方に node SG が入った
- bootstrapper が `ConfigMap` + `ClusterResourceSet` を作成し、workload に `NodePool/EC2NodeClass` が入った

### 4) Workload becomes healthy

workload kubeconfig:

- `/tmp/demo-eks-eni-cleanup-20260210104812-kubeconfig-exec`

確認:

- Fargate 上で `karpenter` / `coredns` が Running
- `NodePool`/`EC2NodeClass` が存在
- Karpenter が EC2 node を 1 台起動し join（Bottlerocket）

### 5) Create a dummy orphan ENI (to reproduce the issue)

bootstrapper-managed node SG:

- SecurityGroup CR: `default/demo-eks-eni-cleanup-20260210104812-karpenter-node-sg`
- SG ID: `sg-027fcf70c4c59e5bd`

ダミー ENI:

- `eni-0586b27458d2b2e4c`
- `Attachment: null`, `Status: available`
- tag: `eks:eni:owner=amazon-vpc-cni`

作成コマンド:

```bash
aws ec2 create-network-interface \
  --region ap-northeast-1 \
  --subnet-id subnet-0abf861a64491f95b \
  --groups sg-027fcf70c4c59e5bd \
  --description orphan-eni-cleanup-test \
  --tag-specifications 'ResourceType=network-interface,Tags=[{Key=eks:eni:owner,Value=amazon-vpc-cni},{Key=Name,Value=orphan-eni-cleanup-test}]'
```

### 6) Delete Cluster and observe orphan ENI cleanup

```bash
kubectl -n default delete cluster.cluster.x-k8s.io demo-eks-eni-cleanup-20260210104812
```

bootstrapper logs で `OrphanENICleanup` が出たことを確認:

- `deleted 1 orphan ENIs ... waiting for ENI disappearance`

AWS 側で ENI が消えたことを確認:

- `aws ec2 describe-network-interfaces --network-interface-ids eni-0586b27458d2b2e4c` -> `InvalidNetworkInterfaceID.NotFound`

## Current status (as of writing)

- orphan ENI は bootstrapper により自動削除できた（狙いの動作）
- 一方で node SecurityGroup deletion は ACK EC2 controller 側で `DependencyViolation` が継続している
  - 当初は `in-use` の ENI が 2 つ見えていた（EKS が作る ENI っぽいもの）
  - その後 `group-id=sg-027fcf70c4c59e5bd` の ENI count は 0 になり、EKS cluster も削除完了（NotFound）
  - それでも SG の Delete が `DependencyViolation` のまま（AWS 側の整合性待ち or 別依存の可能性）

## Next actions

- まず ACK EC2 controller の再試行で SG が消えるか時間経過を観測
- 長時間残る場合:
  - 依存物を追加で棚卸し（SG reference / endpoint ENI / 残存 attachment 等）
  - 必要なら break-glass で SG の依存物を特定・削除し、恒久対策の要件にフィードバック

## Update (2026-02-10)

### 7) Observe node SecurityGroup deletion (ACK EC2)

ACK EC2 controller は `DeleteSecurityGroup` をリトライし続け、最終的に SG が削除された。

- ACK EC2 controller logs:
  - `2026-02-10T02:16:48Z` `DependencyViolation: resource sg-027fcf70c4c59e5bd has a dependent object`
  - `2026-02-10T02:38:44Z` `deleted resource` (SecurityGroup CR: `default/demo-eks-eni-cleanup-20260210104812-karpenter-node-sg`)

AWS 側の確認:

```bash
aws ec2 describe-security-groups --region ap-northeast-1 --group-ids sg-027fcf70c4c59e5bd
# -> InvalidGroup.NotFound
```

### 8) Post-check (no leftovers)

```bash
kubectl get clusters.cluster.x-k8s.io -A
kubectl -n default get clusters.eks.services.k8s.aws
kubectl -n default get securitygroups.ec2.services.k8s.aws
aws eks describe-cluster --region ap-northeast-1 --name demo-eks-eni-cleanup-20260210104812-chj28
aws ec2 describe-network-interfaces --region ap-northeast-1 --filters Name=tag:Name,Values=orphan-eni-cleanup-test
```

- CAPI Cluster / ACK EKS Cluster / ACK EC2 SecurityGroup: none
- AWS EKS Cluster: `ResourceNotFoundException`
- test ENI (tag `Name=orphan-eni-cleanup-test`): 0

## Result

- orphan ENI は bootstrapper の finalizer で自動削除できた
- node SecurityGroup の `DependencyViolation` は発生したが、追加の break-glass 無しで最終的に削除できた（AWS 側の整合性待ちの可能性が高い）

## Re-run (2026-02-10)

同様の手順を再度実施して、orphan ENI cleanup の再現性を確認した。

### 9) Create another test Cluster

- CAPI Cluster:
  - `default/demo-eks-eni-cleanup-20260210120357`
- node SG:
  - SecurityGroup CR: `default/demo-eks-eni-cleanup-20260210120357-karpenter-node-sg`
  - SG ID: `sg-0cc23a9f6eee557ac`

### 10) Create dummy orphan ENI and delete Cluster

- dummy ENI:
  - `eni-09eb1eefd9343aaab`
  - `Attachment: null`, `Status: available`
  - tag `eks:eni:owner=amazon-vpc-cni`

delete:

```bash
kubectl -n default delete cluster.cluster.x-k8s.io demo-eks-eni-cleanup-20260210120357
```

bootstrapper logs:

- `OrphanENICleanup`: `deleted 1 orphan ENIs ... waiting for ENI disappearance`

AWS:

```bash
aws ec2 describe-network-interfaces --region ap-northeast-1 --network-interface-ids eni-09eb1eefd9343aaab
# -> InvalidNetworkInterfaceID.NotFound
```

### 11) Observe deletions reaching completion

- node SG は一時 `DependencyViolation` になったが最終的に削除された
  - `aws ec2 describe-security-groups --region ap-northeast-1 --group-ids sg-0cc23a9f6eee557ac` -> `InvalidGroup.NotFound`
- ACK EKS controller の `DeleteCluster` は FargateProfile が `DELETING` の間 `ResourceInUseException` になったが、解消後にクラスタ削除が進んだ
  - `aws eks describe-cluster --region ap-northeast-1 --name demo-eks-eni-cleanup-20260210120357` -> `ResourceNotFoundException`
