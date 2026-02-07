# Issue: 既存 VPC/Subnet(BYO network) で EKS ControlPlane を作るときの InfrastructureCluster の責務

- 作成日: 2026-02-06
- ステータス: Open (実装済み / manual validation pending)

## 背景

Kany8s は Cluster API-facing な provider suite として:

- Infrastructure: `Kany8sCluster` (`infrastructure.cluster.x-k8s.io`)
- ControlPlane: `Kany8sControlPlane` (`controlplane.cluster.x-k8s.io`)

を提供し、provider-specific な具象化は kro RGD + 下位 controller(例: AWS ACK) に委譲する方針を採っている。

EKS smoke test (`docs/eks/README.md`) では、`Kany8sControlPlane` が参照する RGD の中で VPC/Subnet を新規作成してから EKS Cluster(Control Plane)を作る。

## 問題

実運用では「VPC/Subnet は既に存在し、その上に EKS ControlPlane だけ作りたい」ケースが多い。

このとき Cluster API 的には VPC/Subnet はクラスタ全体の共有インフラであり、通常は `Cluster.spec.infrastructureRef` 側がそれを表現する。

一方で Kany8s は `docs/adr/0008-infra-outputs-policy-parent-rgd-approach-a.md` により、infra -> control plane の値受け渡しを CRD 間で一般化しない方針(Parent RGD で閉じる)を採っているため、以下の設計が曖昧になりやすい:

- 既存 subnet IDs / security group IDs をどこに置くべきか
- `InfrastructureReady` を「既存ネットワークの準備完了」として意味のある状態にするのか
- `Kany8sControlPlane` が EKS 作成に必要なネットワーク情報をどう受け取るべきか(重複/注入/テンプレートパッチ)

## ゴール

- 既存 VPC/Subnet を利用した EKS ControlPlane 作成の推奨パターンを明確化する
- `Cluster` を使う場合、`InfrastructureReady`/`ControlPlaneAvailable` の意味が破綻しないようにする
- Kany8s の「thin provider / provider-agnostic」方針を崩さずに、ユーザ入力の重複/事故を減らす
- Cleanup 時に既存 VPC/Subnet を誤って削除しない

## Non-goals

- 既存 VPC/Subnet の自動検証を完全に行うこと(最初から強い validation を必須にしない)
- NodeGroup / MachinePool まで含めた完全な EKS provider を Kany8s で実装すること
- 既存 AWS リソースの “adoption/import” をこの issue で確定実装すること(必要なら別 issue)

## 設計上の論点

- 責務分担
  - VPC/Subnet は `Kany8sCluster`(Infrastructure) が担うべきか、`Kany8sControlPlane` 側(RGD)に寄せるべきか
- CAPI readiness
  - `Kany8sCluster.status.initialization.provisioned` を何の完了として扱うか(= `Cluster.InfrastructureReady`)
- 入力の重複
  - subnet IDs 等を `Kany8sCluster` と `Kany8sControlPlane` の両方に書く必要があるか
  - ClusterClass/Topology の variables/patch で 1 source of truth にできるか
- `docs/adr/0008-...` との整合
  - infra -> control plane の値の受け渡しを「(計算された)outputs」ではなく「ユーザ入力の共有」として扱うなら許容できるか
- 削除ポリシー
  - BYO network の場合、`kubectl delete` で AWS の subnet/vpc を消してしまう設計にならないか

## 方向性（候補）

### Option 1: 現状維持(ネットワークは ControlPlane RGD 側) + `Kany8sCluster` は stub

- 既存 subnet IDs は `Kany8sControlPlane.spec.kroSpec` に直接渡す
- `Kany8sCluster` は `Ready=True`/`Provisioned=True` の stub のまま

Pros:

- `docs/adr/0008-...` に忠実(値渡しは RGD 内 / もしくは不要)
- 実装コストが低い

Cons:

- `InfrastructureReady` が実態(ネットワーク準備)と乖離しやすい
- worker を後で足す設計に拡張しにくい

### Option 2: BYO network は `Kany8sCluster` に吸収し、入力重複は Topology の patch で解消

- `Kany8sCluster` と `Kany8sControlPlane` の両方が subnet IDs 等を受け取る(ランタイム注入はしない)
- ClusterClass/Topology を推奨し、variables/patch で同一値を双方に流す
- `Kany8sCluster` は「ネットワーク入力が揃った」ことを `Provisioned=True` として表現(強い検証は段階導入)

Pros:

- `InfrastructureReady` の意味が明確(少なくとも “ユーザがネットワーク入力を確定させた” を表せる)
- Controller は provider-agnostic のまま(出力共有/マージ不要)

Cons:

- 低レベル(manifest直書き)では入力が二重になりやすい
- “ネットワークが本当に有効か” は最終的に EKS 作成失敗で分かる可能性がある

### Option 3: `Kany8sControlPlane` が owner `Cluster` の `infrastructureRef` を読んで kroSpec を注入/マージ

- subnet IDs 等は `Kany8sCluster` 側だけを source of truth にする
- `Kany8sControlPlane` は owner `Cluster` を解決し、`Kany8sCluster.spec` から必要値を backend spec に注入する

Pros:

- 入力重複が最小
- CAPI の “infra -> control plane” の直感に近い

Cons:

- `docs/adr/0008-...` の方針と衝突しやすい(「値渡し」の一般化)
- JSON merge の仕様/優先順位/事故(意図せぬ上書き)を慎重に設計する必要がある

## 提案（この issue の結論候補）

まずは Option 2 を推奨パターンとして整備し、必要に応じて Option 3 を検討する。

- 初期は「値の一元化は Topology/patch で行う」ことで Kany8s core の責務増を避ける
- その上で “入力二重が許容しがたい” という UX 要求が強い場合に、注入/マージ(Option 3)を ADR で再検討する

## 成果物（DoD）

- BYO network の推奨 manifest/テンプレ(例)を用意する
  - `Kany8sCluster` 側に subnet IDs を置く例
  - `Kany8sControlPlane` 側にも同値を渡す例(Topology patch 例を優先)
- `InfrastructureReady` の定義(このケースでの意味)をドキュメント化する
- 既存 VPC/Subnet を削除しないための運用ガード(注意書き/retain 方針)を明記する

## 参考

- EKS smoke: `docs/eks/README.md`
- BYO design: `docs/eks/byo-network/design.md`
- Infra outputs policy: `docs/adr/0008-infra-outputs-policy-parent-rgd-approach-a.md`
- Normalized status contract: `docs/adr/0002-normalized-rgd-instance-status-contract.md`
- Spec injection: `docs/adr/0003-kro-instance-lifecycle-and-spec-injection.md`
- Parent RGD pattern (historical): `docs/archive/patterns/parent-rgd-includes-infra.md`

## 実装済み成果物

- `docs/eks/byo-network/manifests/aws-byo-network-rgd.yaml`
- `docs/eks/byo-network/manifests/eks-control-plane-byo-rgd.yaml`
- `docs/eks/byo-network/manifests/clusterclass-eks-byo.yaml`
- `docs/eks/byo-network/manifests/cluster.yaml.tpl`
- `docs/eks/byo-network/README.md`
- `docs/eks/byo-network/design.md` (manifest path を明示)
- `docs/eks/README.md` (BYO セクション追加)
- `docs/eks/values.md` (BYO 必須値と topology 変数対応を追加)
- `docs/eks/cleanup.md` (BYO 削除セマンティクスを明記)

## クローズ条件

以下が実施できたらステータスを `Closed` に更新する:

- BYO RGD 2本が `ResourceGraphAccepted=True`
- `clusterclass-eks-byo.yaml` が apply され、Topology Cluster が期待どおり `Kany8sCluster` / `Kany8sControlPlane` を生成
- Ready 判定（infra input-gate / control plane ACTIVE+endpoint）が設計どおり
- `kubectl delete cluster` 後に EKS/IAM は削除され、既存 VPC/Subnet は不変
