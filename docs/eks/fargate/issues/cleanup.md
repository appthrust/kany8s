# Issue: cleanup stuck by orphan ENI (SecurityGroup DependencyViolation)

EKS Fargate + Karpenter の削除フローで、bootstrapper が作成した node 用 SecurityGroup が消えずに残ることがあります。
本質は「SecurityGroup を参照する orphan ENI が残り、AWS 側で SG の削除が `DependencyViolation` になる」問題です。

## Symptoms (what you see)

- `securitygroups.ec2.services.k8s.aws/<name>` に `deletionTimestamp` が付いたまま消えない
- ACK EC2 controller logs に次が出る
  - `DeleteSecurityGroup ... DependencyViolation: resource sg-... has a dependent object`
- `kubectl -n <ns> describe securitygroups.ec2.services.k8s.aws/<name>` の `Status.conditions` に次が出る
  - `ACK.Recoverable=True` / message に `DependencyViolation`

AWS 側で依存物を確認すると、SG を参照する ENI が残っていることが多いです。

```bash
# SG にぶら下がる ENI を列挙
aws ec2 describe-network-interfaces \
  --region "$AWS_REGION" \
  --filters Name=group-id,Values="$SG_ID" \
  --query 'NetworkInterfaces[].{id:NetworkInterfaceId,status:Status,attachment:Attachment,desc:Description,tags:TagSet}' \
  --output yaml
```

典型例:

- `Description: aws-K8S-i-...`
- tags:
  - `eks:eni:owner=amazon-vpc-cni`
  - `node.k8s.amazonaws.com/instance_id=i-...`
  - `cluster.k8s.amazonaws.com/name=<eksClusterName>`
- `Status: available` かつ `Attachment: null`（未アタッチのまま残骸化）

## Root cause (why it happens)

- Karpenter node で動く `amazon-vpc-cni` が ENI を作成する（pod ENI / secondary ENI など）
- クラスタ削除や node 強制終了のタイミング次第で、ENI の削除が追従せず orphan ENI が残ることがある
- orphan ENI が node 用 SecurityGroup を参照し続けるため、SG の削除が AWS 側で `DependencyViolation` になる

この orphan ENI は「いつか消えることもある」が、時間が読めず（総合試験/開発リセットで）削除の確実性を下げます。

## Manual remediation (break-glass / one-off)

前提: **クラスタ削除中または削除完了後** で、対象 node が残っていないこと。
（稼働中の node の ENI を消すと通信断になります）

```bash
export AWS_REGION=ap-northeast-1
export NAMESPACE=default
export SG_CR_NAME=<securitygroup-cr-name>

# 1) SG ID を取得（ACK CR の status.id）
SG_ID="$(kubectl -n "$NAMESPACE" get securitygroups.ec2.services.k8s.aws/$SG_CR_NAME -o jsonpath='{.status.id}')"

# 2) 依存 ENI を列挙
ENI_IDS="$(aws ec2 describe-network-interfaces \
  --region "$AWS_REGION" \
  --filters Name=group-id,Values="$SG_ID" \
  --query 'NetworkInterfaces[].NetworkInterfaceId' --output text)"

# 3) orphan ENI（Status=available, Attachment=null）だけを削除
for eni in $ENI_IDS; do
  aws ec2 describe-network-interfaces \
    --region "$AWS_REGION" \
    --network-interface-ids "$eni" \
    --query 'NetworkInterfaces[0].{id:NetworkInterfaceId,status:Status,attachment:Attachment}' \
    --output yaml

  aws ec2 delete-network-interface --region "$AWS_REGION" --network-interface-id "$eni"
done

# 4) ACK 側の SG CR が消えるのを待つ（ACK controller が再試行して収束する想定）
kubectl -n "$NAMESPACE" wait --for=delete --timeout=5m securitygroups.ec2.services.k8s.aws/$SG_CR_NAME
```

## Proposed permanent fix (recommended)

`eks-karpenter-bootstrapper` の delete/finalizer フローに「orphan ENI cleanup」を組み込む。

- **いつ実行するか**
  - `Cluster` が delete 中（`DeletionTimestamp != nil`）のときのみ
  - bootstrapper が管理する node SG の削除が `DependencyViolation` で詰まっているとき
- **何を消すか（安全ゲート）**
  - node SG を参照する ENI を列挙し、次だけを対象にする
    - `Attachment == nil`（未アタッチ）
    - `Status == available`
    - tags/description が `amazon-vpc-cni` 起因であること（例: `eks:eni:owner=amazon-vpc-cni`, `aws-K8S-i-...`）
    - 可能なら `node.k8s.amazonaws.com/instance_id` が「bootstrapper が terminate 済み」または `terminated` であること
- **実装方針**
  - AWS API: `DescribeNetworkInterfaces` + `DeleteNetworkInterface`
  - backoff + requeue で収束（削除が遅延しても最終的に取りこぼしにくくする）
  - Event/metric で可視化（削除した ENI 数、削除待ち理由など）
- **必要権限（AWS）**
  - `ec2:DescribeNetworkInterfaces`
  - `ec2:DeleteNetworkInterface`
  - （既存の instance cleanup に加えて）

併せて `hack/eks-fargate-dev-reset.sh` は、`DependencyViolation` 発生時に
SG 依存 ENI の一覧と orphan ENI の break-glass delete コマンドを表示します
（デフォルトでは実行しない）。

## Design (this repo)

この repo では上記の恒久対応を、`eks-karpenter-bootstrapper` の Cluster delete finalizer に組み込みます。

- trigger: `Cluster.metadata.deletionTimestamp != nil` かつ finalizer `eks.kany8s.io/karpenter-cleanup` が残っている間
- order:
  1) Flux `OCIRepository`/`HelmRelease` suspend
  2) workload 側 Karpenter provisioning stop
  3) Karpenter nodes (EC2) terminate（discovery tag）
  4) orphan ENI cleanup（本 issue）
  5) finalizer remove
- target SecurityGroup:
  - auto-create 時に作る ACK `SecurityGroup` のみを対象にする
  - `securitygroups.ec2.services.k8s.aws/<capiClusterName>-karpenter-node-sg` かつ `eks.kany8s.io/managed-by=eks-karpenter-bootstrapper`
- candidate ENI:
  - `group-id == <node SG id>` を参照
  - `Attachment == null`（未アタッチ）
  - `Status == available`
  - tag `eks:eni:owner=amazon-vpc-cni`
- behavior:
  - candidate があれば `DeleteNetworkInterface` を実行し、次の reconcile で candidate が 0 になるまで requeue
  - 失敗時は `OrphanENICleanup` Event + logs に理由を出し、必要権限/手動手順へ誘導

実装参照:

- `internal/controller/plugin/eks/karpenter_bootstrapper_controller.go`
- `internal/controller/plugin/eks/karpenter_bootstrapper_orphan_eni_cleanup.go`
