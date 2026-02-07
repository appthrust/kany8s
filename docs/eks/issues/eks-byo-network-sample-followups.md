# Issue: EKS BYO Network サンプルの次アクション

- 作成日: 2026-02-07
- ステータス: Open

## 背景

`docs/eks/byo-network/` の手順で以下ができている:

- ACK(EC2) で VPC/Subnet を bootstrap し、subnet IDs を取得できる
- ClusterClass/Topology で `Kany8sCluster` + `Kany8sControlPlane` を生成し、ACK(EKS/IAM) で EKS ControlPlane を作れる

一方で、サンプルを「運用/検証に使える」状態にするには未整備の点が残っている。

## ゴール

- BYO サンプルの利用者が迷わない（入力・観測・削除の UX を固める）
- 可能であれば CAPI `Cluster` の `Available=True` まで到達させる（少なくとも到達できない理由と回避策を明示）
- bootstrap network で作った VPC/Subnet を誤削除しない / 消すときは手順が明確

## 課題 / 次にやること

- [x] Kubeconfig の扱いを決める（ユーザが kubectl で接続できること）
- [x] CAPI の `RemoteConnectionProbe` を通す方針を決める
- [ ] BYO サンプルの cleanup（EKS/IAM と network bootstrap の関係）を明確化する
- [ ] ClusterClass の naming を固定してリソース名のランダム suffix をなくす（手順と観測コマンドの簡略化）
- [ ] `docs/eks/byo-network/` を end-to-end で完結する手順にする（`docs/eks/README.md` 参照を最小化）

## 詳細 TODO

### 1) Kubeconfig / RemoteConnectionProbe

- [x] 現状の挙動を整理する
- [x] `Kany8sControlPlane` の kubeconfig Secret 対応方針を決める
  - 採用: BYO では `status.kubeconfigSecretRef` を使わず、plugin が `<cluster>-kubeconfig` を直接管理する
- [x] EKS の認証方式（IAM token）を踏まえ、remote probe をどう成立させるかの方針を確定する
  - 採用: probe 用は token 埋め込み kubeconfig をローテーションし、人間用は `exec` kubeconfig（`<cluster>-kubeconfig-exec`）を分離して提供
- [ ] 上記方針を plugin 実装/検証（kind + ACK + CAPI）へ落とし込む

### 2) bootstrap network の cleanup

- [ ] `docs/eks/byo-network/manifests/bootstrap-network.yaml.tpl` で作った VPC/Subnet の削除手順を追加する
  - どのリソースを消すと何が消えるか（EKS/IAM と VPC/Subnet の独立性）
  - 削除の順序（Subnet -> VPC など）と `kubectl wait --for=delete` の例

### 3) ClusterClass naming の固定

- [ ] `docs/eks/byo-network/manifests/clusterclass-eks-byo.yaml` に naming template を追加する
  - `spec.infrastructure.naming.template`
  - `spec.controlPlane.naming.template`
  - 目標: `Cluster` 名と `Kany8sCluster` / `Kany8sControlPlane` / kro instance 名の対応を固定し、手順・観測が簡単になる

### 4) ドキュメント整備

- [ ] `docs/eks/byo-network/README.md` に "必要な環境変数" セクションを追加する
  - `KUBERNETES_VERSION` (Topology: semver, 例: `v1.35.0`)
  - `EKS_VERSION` (major.minor, 例: `1.35`)
  - `PUBLIC_ACCESS_CIDR`
- [ ] `docs/eks/byo-network/README.md` に "進捗確認" セクションを追加する（kro instance / ACK resource / Kany8s facade）

## DoD

- [ ] `docs/eks/byo-network/` の README だけで bootstrap -> EKS 作成 -> 状態確認 -> cleanup の導線が読める
- [ ] `kubectl delete cluster.cluster.x-k8s.io <name>` で EKS/IAM が消えることが確認できる
- [ ] VPC/Subnet は BYO(shared infra) として残ること、消す場合の手順が明記されている
- [ ] `Available=True` を狙う場合はその実現手段がある / 難しい場合は理由と回避策が明記されている
