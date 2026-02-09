# Plan: EKS BYO + Fargate bootstrap + Karpenter (no NodeGroup)

- 更新日: 2026-02-08
- スコープ: `docs/eks/fargate/`（BYO network サンプルを「手作業なし」に寄せる）

## 何を達成したいか

- ユーザーの操作を「YAML apply + opt-in」だけにする
- `aws ...` / `kubectl patch ...` のような手作業(command ops)を不要にする
- FargateProfile/AccessEntry/IAM/OIDC/Flux/CRS 等は management cluster 上の CustomResource (ACK/Flux/CAPI) と controller で収束させる

## 現状 (この repo の状態)

- `eks-kubeconfig-rotator` は実装済み（`<cluster>-kubeconfig` を token 埋め込みでローテーション）
- `eks-karpenter-bootstrapper` は実装済み（OIDC/Role/Policy/AccessEntry/FargateProfile/Flux/CRS/ConfigMap を生成）
- node 用 SecurityGroup は command ops なしで収束する（bootstrapper が ACK `SecurityGroup` 作成 + Topology variable 注入）
  - 過去の手作業ログ: `docs/eks/fargate/wip.md`

## 目指す UX (最終)

1) management cluster(kind) に kro/ACK/Kany8s (+ plugins) を入れる
2) BYO ClusterClass + RGD を apply（1回）
3) Cluster(Topology) を apply（1回）
4) Cluster に opt-in を付与（または ClusterClass から自動注入）
5) 待つだけで次が成立する
   - workload: `kube-system/coredns` が Fargate で Running
   - workload: `karpenter/*` が Fargate で Running
   - workload: NodePool/EC2NodeClass が適用され node が join する

## 実装プラン

### A) node SecurityGroup を CR で自動作成 (command ops 削除の本丸)

- 条件: `eks.kany8s.io/karpenter=enabled` かつ `vpc-security-group-ids` が空
- management cluster に `ec2.services.k8s.aws/v1alpha1 SecurityGroup` を作成する
- SG の最小ルール（案）
  - ingress: self all
  - egress: `0.0.0.0/0` all
- SG を EKS と node の両方で使う
  - bootstrapper が `Cluster.spec.topology.variables["vpc-security-group-ids"]` を patch して SG ID を注入する
  - 期待する効果
    - ACK EKS Cluster が `resourcesVPCConfig.securityGroupIDs` を更新し、control plane ENI に attach される
    - Karpenter の `EC2NodeClass.securityGroupSelectorTerms` に同じ SG ID が入る

VPC ID の解決方法（いずれかを採用）:

1) (推奨) Topology variable で `vpc-id` を追加し、入力として渡す
2) (自動) bootstrapper が AWS API (EC2 DescribeSubnets) を read-only で呼び、subnet IDs -> VPC ID を解決する
3) (自動) BYO ではなく bootstrap network（ACK で VPC/Subnet を作る）を使う場合は、VPC CR の status 参照で解決する

RBAC (bootstrapper):

- `securitygroups.ec2.services.k8s.aws` の create/get/list/watch/update/patch を追加

### B) HelmRelease の信頼性

- install/upgrade の `timeout` を延ばす
- `disableWait` を有効化し、FargateProfile が ACTIVE になる前の wait で Stalled になりにくくする
- `remediation.retries` を入れて「いつか収束」へ寄せる

### C) E2E/Acceptance の再現性

- kind 管理クラスタ上で「BYO + plugins + opt-in」だけで node まで増えることをスクリプト化する
- 成功条件（最小）
  - management: AccessEntry/FargateProfile/IAM/OIDC/Flux/CRS が揃う
  - workload: `coredns` と `karpenter` が Running、node が join

### D) Dev reset (VPC/Subnet も含めて 1 からやり直す)

実装の検証中は、ネットワーク要件（NAT/VPC endpoints）が満たされていない VPC/Subnet を使ってしまい、
Fargate 上の `coredns` / `karpenter` が `ImagePullBackOff` で止まることがある。

この repo の開発中に作った VPC/Subnet を使っていた場合は、いったん **VPC/Subnet も含めて全削除**して
「要件を満たす network」から作り直す（= clean slate）方が早い。

ポイント:

- 先に EKS Cluster を削除し、Fargate profile/ENI などの依存が解消されてから Subnet/VPC を削除する
- management cluster(kind) を先に消さない（ACK finalizer が走らなくなる）

作業ログは `docs/eks/fargate/wip.md` に残す。

### E) IAM InstanceProfile を ACK で明示管理 (cleanup の詰まり解消)

課題:

- Karpenter が node 用 InstanceProfile を自動生成する場合、クラスタ削除時に InstanceProfile が残り
  ACK `Role` deletion が `DeleteConflict` で詰まることがある

方針（ACK-first）:

- `eks-karpenter-bootstrapper` が node 用 `iam.services.k8s.aws/v1alpha1 InstanceProfile` を作成・管理する
  - `spec.name`: `<eksClusterName>-node` 系の安定名（AWS name 制約に収まるよう短縮）
  - `spec.roleRef`: bootstrapper が作成する node role を参照
- ClusterResourceSet で apply する `EC2NodeClass` は `spec.instanceProfile=<InstanceProfile.spec.name>` を使う
  - `spec.role` は設定しない（CRD の mutual exclusive を満たす）

期待効果:

- cleanup 時に InstanceProfile が Kubernetes resource として先に消え得るため、Role deletion が収束しやすい
- 旧挙動（Karpenter auto-generated InstanceProfile）に依存しない

## 参照

- 設計詳細: `docs/eks/fargate/design.md`
- 実装 TODO: `docs/eks/fargate/todo.md`
- 実行ログ: `docs/eks/fargate/wip.md`
