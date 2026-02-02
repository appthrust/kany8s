# PRD: Kany8s

- 作成日: 2026-01-25
- 最終更新: 2026-02-02
- ステータス: Draft (design-first / prototype)

Kany8s は Cluster API (CAPI) と kro (ResourceGraphDefinition / RGD) を組み合わせ、managed Kubernetes control plane (EKS/GKE/AKS 等) と kubeadm による self-managed control plane を、Kubernetes ネイティブに作成・運用するための Cluster API provider suite です。

プロジェクト目標 (Must):

- self-managed: (前提: 管理クラスタに CAPI core + CABPK + infrastructure provider を導入した上で) Kany8s の定義（Kany8s CRD/controller）だけで、少なくとも CAPD(docker) 上に "実際に動作する" Kubernetes クラスタを作成できることを保証する（CAPI の `RemoteConnectionProbe=True` / `Cluster Available=True` まで到達でき、kubeconfig で接続できる）。
- managed(EKS): (前提: 管理クラスタに CAPI core + kro + ACK(IAM/EKS) を導入した上で) CAPI `Cluster` の `controlPlaneRef` として `Kany8sControlPlane` を利用し、RGD を通じて EKS の managed control plane を作成できることを保証する（`Kany8sControlPlane Ready=True` / endpoint 設定まで到達できる）。

本 PRD は Why/What/How を 1 枚にまとめた形で記載します。詳細設計と実装タスクは別ドキュメントへ切り出します（末尾の参照）。

## 1. 概要

### 製品概要

- プロダクト名: Kany8s (k(ro)+any+k8s)
- タグライン: Any k8s, powered by kro.
- 位置付け: Cluster API provider suite
  - `Kany8sControlPlane` (managed control plane / kro)
  - `Kany8sKubeadmControlPlane` (self-managed kubeadm control plane)
  - `Kany8sCluster` (Infrastructure provider)

### 価値提案 (Why)

- CAPI を使ったクラスタライフサイクル管理の枠組みを維持したまま、EKS/GKE/AKS 等の "managed control plane" を同一の操作感で作成できる。
- プロバイダ固有ロジックを controller 実装に持ち込まず、kro RGD を差し替えるだけでマルチクラウド拡張できる。
- "もし kro RGD で表現できるなら Kany8s が CAPI 経由で駆動できる" を実現する。

### 前提 / 用語

- CAPI: Cluster API。`Cluster` / `ClusterClass` / `ClusterTopology` によるクラスタ API。
- kro: Kubernetes-SIGs の resource composition engine。
- RGD: `ResourceGraphDefinition`。kro が生成するカスタム API と、その実体リソース(DAG)定義。
- kro instance: RGD が生成するカスタムリソース(例: `EKSControlPlane`)。
- ACK: AWS Controllers for Kubernetes。EKS/IAM/S3/SQS/EventBridge などを CR で操作する。
- CAPD: Cluster API Provider Docker。Docker 上に self-managed Kubernetes を立てるための infrastructure provider。
- CABPK: Cluster API Bootstrap Provider Kubeadm。`KubeadmConfig` から bootstrap data を生成する。
- KCP: KubeadmControlPlane。kubeadm control plane provider の参照実装。

## 2. 背景

- managed Kubernetes を採用する組織が増え、複数クラウド/複数アカウント/複数環境を横断した "同じやり方" によるクラスタ作成ニーズが強い。
- CAPI はクラスタ管理の共通 API だが、一般的な provider 実装はクラウドごとに controller を実装する必要があり、追加コスト/保守コストが高い。
- 既存の CAPT(Cluster API Provider Terraform) は Terraform Workspace を実行単位にし outputs を Secret に書き出す設計で、Terraform 前提・Template→Apply の運用複雑性がある。
- kro は DAG に基づく合成と status 射影を提供し、"プロバイダ固有の具象化" を RGD 側へ閉じ込められる。

Kany8s は、CAPI の contract を満たす "最小の provider" として振る舞い、具象リソース作成は kro(RGD) に委譲することで、"controller を増やさずに provider を増やす" ことを狙う。

## 3. 製品原則

- Provider-agnostic controller: controller はクラウド固有 CR の status を読まない。kro instance の正規化 status のみを参照する。
- CAPI contract first: endpoint/initialized/conditions 等の contract を最優先に実装し、CAPI の標準フロー(Cluster controller による endpoint 反映等)に乗る。
- RGD を "具象化エンジン" として扱う: provider 固有差分は RGD で吸収し、Kany8s 本体は分岐を持たない。
- 依存関係は RGD に閉じる: infra と control plane の値受け渡しが必要な場合は、ControlPlane が参照する "親 RGD" に infra も含め、RGD chaining/DAG 内で完結させる。Kany8s の CR 間で汎用 outputs を受け渡す仕組みは導入しない。
- 小さく分割・合成: "巨大な 1 枚 RGD" を避け、部品 RGD + chaining で再利用可能にする。
- Ready の定義を守る: ControlPlane の Ready は "API endpoint を設定できる" を意味し、Addon 等の周辺リソースの ready とは分離する。
- Secrets は最小限: kubeconfig Secret 等、CAPI contract 上必須なもの以外は "汎用 outputs" を原則持ち込まない。
- Working cluster first: CAPD(docker) をリファレンス環境として、`RemoteConnectionProbe=True` / `Cluster Available=True` を "成功" の最低条件に含める。

## 4. スコープ

### MVP (現状)

- managed control plane (kro)
  - `Kany8sControlPlane` は RGD を選択し、kro instance を 1:1 で作成/更新する
  - kro instance の `status.ready` / `status.endpoint` / `status.kubeconfigSecretRef` を消費し、CAPI contract を満たす状態へ反映する
- self-managed kubeadm
  - `Kany8sKubeadmControlPlane` は CAPD + CABPK と組み合わせて、kubeadm ベースの control plane を成立させる
- infrastructure
  - `Kany8sCluster` は CAPI の `Cluster.spec.infrastructureRef` を満たす
  - 現状は "stub infra provider" として `status.initialization.provisioned=true` を立てて unblock するのみ

### プロジェクト目標 (Must): CAPD(docker) + kubeadm で self-managed Kubernetes を作れること

- Kany8s が `KubeadmControlPlane` 相当の責務を持ち、control plane Machines を作成/管理できる。
- CABPK と連携し、`KubeadmConfig` を通じて bootstrap data を生成できる。
- kubeadm に必要なクラスタ証明書（CA/front-proxy/SA key 等）を生成・管理し、bootstrap 生成に供給できる。
- `<cluster>-kubeconfig` Secret を生成・維持し、CAPI の `RemoteConnectionProbe` が成功することを保証する。
- endpoint の source of truth を明確化する（CAPD の LB endpoint を優先し、Kany8s が誤って上書きしない）。
- 受け入れ条件: `Cluster Available=True` になり、kubeconfig で workload cluster に接続できる（例: `kubectl get nodes` が成功）。

### プロジェクト目標 (Must): EKS で managed control plane を作れること

- 前提: 管理クラスタに CAPI core + kro + ACK(IAM/EKS) + Kany8s がインストール済みであること（クラウド認証情報は ACK の標準方式に委譲）。
- CAPI `Cluster` の `controlPlaneRef` に `Kany8sControlPlane` を指定し、kro RGD（例: `eks-control-plane.kro.run` / `eks-platform-cluster.kro.run`）を駆動して EKS control plane を作成できる。
- 受け入れ条件: `Kany8sControlPlane Ready=True` になり、endpoint が設定される（kro instance の `status.ready=true` / `status.endpoint != ""` を満たす）。
- 備考: EKS については本フェーズでは `RemoteConnectionProbe=True` / `Cluster Available=True` の達成を必須条件に含めない（後続で扱う）。

### スコープ外 (現時点でやらない)

- CAPT の Template→Apply パターンを中核概念として再現しない
- Terraform の outputs のような "汎用 outputs → Secret" をコア設計として採用しない
- ControlPlane Ready に Addon/S3/SQS/EventBridge 等の周辺 ready を含めない(必要なら別 RGD へ)
- Worker(MachineDeployment/MachinePool) の自動作成・管理（当面）

### 未決 / Planned

- `Kany8sCluster` を kro(RGD) と連携して "infra も具象化する provider" に拡張する
  - `spec.resourceGraphDefinitionRef` を導入し、kro instance の `status.ready/reason/message` を `status.initialization.provisioned` に反映する
  - ただし infra の出力（例: VPC ID 等）を control plane に渡す仕組みは `Kany8sCluster` には持ち込まない（案A）。必要な場合は ControlPlane が参照する "親 RGD" に infra を含め、RGD chaining/DAG 内で完結させる。
  - 再利用 VPC 等を使う場合は、値を `Kany8sControlPlane.spec.kroSpec` / Topology variables から入力として渡す

## 5. 対象ユーザー

- Platform Engineer / SRE: CAPI を採用した管理クラスタ上で、managed control plane をセルフサービス化したい
- 組織内の "クラスタ提供者": マルチクラウド(将来)を見据え、provider 追加を "コードではなく RGD" で行いたい
- CAPI ユーザー: 既存の CAPI ワークフロー(clusterctl / GitOps / ClusterClass)に managed Kubernetes を統合したい

## 6. ユースケース

- UC0: CAPD(docker) で self-managed Kubernetes(kubeadm) を作る
  - Kany8s を ControlPlane provider として利用し、CAPI の `RemoteConnectionProbe`/`Available` まで到達する "基準パス" とする
- UC1: CAPI `Cluster` から managed control plane を作る
  - `Cluster.spec.controlPlaneRef` に `Kany8sControlPlane` を指定し、RGD を参照して EKS 等を作成する
- UC2: ClusterClass/Topology による標準化/セルフサービス (planned)
  - `Cluster.spec.topology.version` を単一ソースとして、kro instance `spec.version` へ注入する
  - 変数(patches/variables)で `kroSpec` を供給し、環境ごとの差分(リージョン/VPC 等)を吸収する
- UC3: provider 追加/切り替え
  - controller は変更せず、RGD を追加(例: `gke-control-plane`)して `resourceGraphDefinitionRef` を切り替える

## 7. 市場分析

- "Kubernetes クラスタ管理" の手段は Terraform/各クラウド API/専用 SaaS 等に分散しており、組織が複数環境を抱えるほど運用コストが増える。
- GitOps / Kubernetes-native な運用が浸透し、"クラスタ作成" 自体も CRD/controller で完結させたい需要が増えている。
- 一方で、CAPI provider をクラウドごとに実装・保守するコストは依然高く、マルチクラウド/独自基盤の拡張が難しい。

Kany8s は、CAPI の共通 API を "入口" に保ちつつ、具象化を kro(RGD) に寄せることで "拡張コストの構造" を変える。

## 8. 競合分析

- CAPI ネイティブ provider (例: CAPA/CAPZ/CAPG)
  - 強み: production 実績/機能が豊富
  - 弱み: provider 追加や独自基盤対応は controller 実装が必要
- CAPT (Terraform)
  - 強み: Terraform により広範なリソースを扱える
  - 弱み: Terraform 前提/Template→Apply の運用コスト/outputs 連携の複雑性
- Crossplane (Providers)
  - 強み: Kubernetes でのインフラ管理・合成が強い
  - 弱み: CAPI contract を前提にしていない(Cluster API のワークフローと別軸)
- kro 単体
  - 強み: DAG + 合成
  - 弱み: CAPI contract を満たす control plane provider ではない

差別化:

- "CAPI contract" を満たす薄い provider を提供しつつ、"具象化は RGD" で差し替え可能にする。

## 9. 機能要求

優先度は Must/Should/Could で記載します。

### 9.1 API/CRD

- Must: `Kany8sControlPlane` CRD
  - spec
    - `spec.version` (required)
    - `spec.resourceGraphDefinitionRef.name` (required)
    - `spec.kroSpec` (optional, provider-specific object)
    - `spec.controlPlaneEndpoint` (controller が設定)
  - status
    - `status.initialization.controlPlaneInitialized`
    - `status.conditions`
    - `status.failureReason` / `status.failureMessage` (terminal error のみ)

- Must: `Kany8sKubeadmControlPlane` CRD (self-managed kubeadm)
  - KCP 互換の最小 API として、replicas/machineTemplate/kubeadmConfigSpec/conditions を持つ

- Must: `Kany8sCluster` CRD
  - 最低限: CAPI v1beta2 InfrastructureCluster contract の `status.initialization.provisioned` を満たす
  - 追加目標: `spec.resourceGraphDefinitionRef` + `spec.kroSpec` を導入し、kro instance による infra 具象化へ拡張

- Could: `Kany8sControlPlaneTemplate` / `Kany8sClusterTemplate` (ClusterClass 利用)

### 9.2 Controller 振る舞い

- Must: `Kany8sControlPlane` reconcile
  - 参照 RGD を取得し、生成される instance の GVK を解決できる
  - kro instance を 1:1 で作成/更新できる (OwnerReference 付与)
  - kro instance へ `spec.version` を必ず注入(上書き)できる
  - kro instance の `status.ready/endpoint` を読み、CAPI contract に従って endpoint/initialized/conditions を更新できる

- Must: self-managed kubeadm control plane provisioning (`Kany8sKubeadmControlPlane`)
  - control plane Machines を作成/更新/削除できる（`Machine` と infra/bootstrap 参照を含む）
  - kubeadm に必要な証明書と kubeconfig Secret を生成・維持できる
  - CAPD の `DockerCluster.spec.controlPlaneEndpoint` と矛盾しない endpoint 運用を行う
  - 受け入れ条件: `RemoteConnectionProbe=True` / `Cluster Available=True`（NoWorkers は許容）

- Must: `Kany8sCluster` reconcile (stub)
  - `status.initialization.provisioned=true` を設定し、`Ready=True` を立てる

- Should: `Kany8sCluster` reconcile (kro 連携)
  - `spec.resourceGraphDefinitionRef` で RGD を選択
  - kro instance を 1:1 で作成/更新できる
  - kro instance の `status.ready/reason/message` を読み、`status.initialization.provisioned` と Conditions を更新できる

### 9.3 kro instance status 正規化 contract

ControlPlane (managed control plane / kro instance):

- Must: `status.ready: boolean` (ControlPlane ready)
- Must: `status.endpoint: string` (`https://host[:port]` or `host[:port]`)
- Should: `status.reason: string`
- Should: `status.message: string`
- Should: `status.kubeconfigSecretRef` (name/namespace)

Infrastructure (kro 連携を行う場合):

- Must: `status.ready: boolean` (Infrastructure ready)
- Should: `status.reason: string`
- Should: `status.message: string`

備考:

- kro は `status.conditions` / `status.state` を予約しているため、上記は専用フィールド名で持つ。

### 9.4 インストール/配布

- Should: install manifests (RBAC/Deployment/CRD) の提供
- Could: Helm chart / clusterctl provider packaging

## 10. UX要求, UIモックアップ

Kany8s の UX は "YAML + kubectl/clusterctl" を中心とする。

### 10.1 ユーザーフロー(例: 直指定)

1. kro を管理クラスタにインストール
2. Kany8s を管理クラスタにインストール
3. provider-specific RGD(例: `eks-control-plane`) を apply
4. `Cluster` + `Kany8sControlPlane` を apply
5. `kubectl get clusters` / `kubectl get kany8scontrolplanes` で Ready と endpoint を確認

### 10.2 YAML モック

`Cluster`:

```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: demo-cluster
spec:
  infrastructureRef:
    apiGroup: infrastructure.cluster.x-k8s.io
    kind: Kany8sCluster
    name: demo-cluster
  controlPlaneRef:
    apiGroup: controlplane.cluster.x-k8s.io
    kind: Kany8sControlPlane
    name: demo-cluster
```

`Kany8sControlPlane`:

```yaml
apiVersion: controlplane.cluster.x-k8s.io/v1alpha1
kind: Kany8sControlPlane
metadata:
  name: demo-cluster
spec:
  version: "1.34"
  resourceGraphDefinitionRef:
    name: eks-control-plane
  kroSpec:
    region: ap-northeast-1
    vpc:
      subnetIDs: ["subnet-xxxx", "subnet-yyyy"]
      securityGroupIDs: ["sg-zzzz"]
```

### 10.3 期待される表示(例)

- `kubectl get kany8scontrolplanes`
  - READY/INITIALIZED/ENDPOINT が分かる(Conditions でも可)
- `kubectl describe kany8scontrolplane demo-cluster`
  - `status.conditions` に Creating/Ready、失敗時は Reason/Message が出る

## 11. 技術的要求 (システム要求、セキュリティ要求、プライバシー要求、パフォーマンス要求)

### 11.1 システム要求

- 管理クラスタ上で動作する Kubernetes controller として提供する
- CAPI v1beta2 `Cluster` と ControlPlane provider contract に準拠する
- RGD は cluster-scoped 前提、kro instance は参照元 CR と同一 namespace に作成する
- self-managed(kubeadm) の場合は、管理クラスタに CAPI core + CABPK + infrastructure provider (例: CAPD) がインストール済みであることを前提に、Kany8s が control plane provider としてそれらを組み合わせる

### 11.2 kro 実装制約への適合 (kro v0.7.1 検証より)

- `spec.schema.status` の CEL では `schema.*` を参照できない
- `readyWhen` は self resource しか参照できない
- status の文字列テンプレートはリテラル欠落が起こり得るため CEL 1 式で連結する
- status field は resource 参照を含まないと reject される(定数が置けない)
- `NetworkPolicy` を含む graph が Ready にならない等の既知問題があるため、Kany8s 付属/推奨 RGD では避ける

### 11.3 セキュリティ要求

- controller の RBAC は最小権限を基本としつつ、動的 GVK(kro instance)作成に必要な権限を設計に落とす
- クラウド認証情報は Kany8s が保持せず、下位 controller(例: ACK) の標準的な credential 管理に委譲する
- kubeconfig 等の Secret を扱う場合、ログ出力しない/マスクする

### 11.4 プライバシー要求

- 個人情報(PII)の取り扱いを前提としない
- 監査/ログはクラスタ名・namespace・RGD 名など運用に必要な最小情報に限定する

### 11.5 パフォーマンス要求

- 1 cluster = 1 kro instance の最小監視モデルを前提に、watch 対象を絞る
- `status.ready/endpoint` の変化に追従できる reconcile 周期/イベント駆動を採用する
- 大量クラスタでも controller が過剰な list/watch を行わない

## 12. リリーススケジュールおよびマイルストーン

日付は目安。実装・検証の結果により変更する。

- M0: PRD/設計確定
- M1: `Kany8sControlPlane` MVP
- M2: 参照 RGD カタログ + ドキュメント
- M2b: CAPD(docker) + kubeadm で "実動作クラスタ" を作る
  - DoD: `RemoteConnectionProbe=True` / `Cluster Available=True`、kubeconfig で接続できる
- M3: `Kany8sCluster` を kro(RGD) と連携して infra も具象化できる provider に拡張

## 13. 参照 (詳細設計 / 実装タスク)

- 設計: `docs/design.md`
- RGD contract: `docs/rgd-contract.md`
- RGD guidelines/kro pitfalls: `docs/rgd-guidelines.md`, `docs/kro.md`
- 経緯/詳細(方針の背景): `docs/PRD-details.md`
- 実装タスク: `docs/TODO.md`
- CRD/Domain review (historical): `docs/review.md`
- E2E/Acceptance: `docs/e2e-guide.md`, `docs/e2e-and-acceptance-test.md`

## 14. TODO: Infrastructure (`Kany8sCluster`)

目的: `Kany8sCluster` を "stub infra provider" から、kro(RGD) による infra 具象化ができる provider へ拡張する。

方針:

- MVP は後方互換: `spec.resourceGraphDefinitionRef` が無い場合は現状どおり stub として `provisioned=true` を立てる
- `spec.resourceGraphDefinitionRef` が指定された場合のみ kro 連携を有効化する
- `Kany8sCluster` controller は provider-specific な CR を直接読まず、kro instance の正規化 status のみを読む
- 新方針(案A): infra と control plane の値受け渡しが必要な場合は、`Kany8sControlPlane` が参照する "親 RGD" に infra を含めて完結させる。`Kany8sCluster` は control plane の inputs を生成/配布する役割を持たず、汎用 outputs を導入しない。

### 14.1 Contract (docs)

- [x] infra RGD instance の status 契約を `docs/rgd-contract.md` に追記する
  - 追加: Infrastructure contract `status.ready` / `status.reason` / `status.message`
  - 追加: `Kany8sCluster.status.initialization.provisioned` への反映ルール（`status.ready=true` -> `provisioned=true`）
  - DoD: RGD 作者が "infra 側は何を出せば良いか" を迷わない

### 14.2 API (CRD)

- [ ] infra API group に `ResourceGraphDefinitionReference` を追加する（ControlPlane と同等の形）
  - Touch: `api/infrastructure/v1alpha1/` (new file or inline)
  - DoD: `spec.resourceGraphDefinitionRef.name` を型として表現できる

- [ ] `Kany8sClusterSpec` に `spec.resourceGraphDefinitionRef` を追加する（kro 連携のスイッチ）
  - Touch: `api/infrastructure/v1alpha1/kany8scluster_types.go`
  - 仕様: 未指定なら stub mode / 指定ありなら kro mode
  - DoD: `make manifests generate test` が通る

- [ ] `Kany8sClusterTemplate` にも `resourceGraphDefinitionRef` を追加し、Topology から同等の入力が渡せるようにする
  - Touch: `api/infrastructure/v1alpha1/kany8sclustertemplate_types.go`
  - DoD: ClusterClass/Topology から生成される `Kany8sCluster` が kro mode に入れる

- [ ] サンプルを更新する（kro mode の最小例を含める）
  - Touch: `config/samples/infrastructure_v1alpha1_kany8scluster.yaml`, `config/samples/infrastructure_v1alpha1_kany8sclustertemplate.yaml`
  - DoD: `make deploy` 後に samples が apply できる

### 14.3 Controller (kro integration)

- [ ] RBAC を追加する（RGD 読み取り + kro instance 作成/更新）
  - Touch: `internal/controller/infrastructure/kany8scluster_controller.go` (+kubebuilder:rbac)
  - DoD: `make manifests` で生成される RBAC に `resourcegraphdefinitions.kro.run` と instance への権限が含まれる

- [ ] kro mode の足場を実装する（RGD -> instance GVK 解決 + instance 1:1 create/update）
  - Touch: `internal/controller/infrastructure/kany8scluster_controller.go`
  - 利用: `internal/kro/gvk.go`
  - DoD: `spec.resourceGraphDefinitionRef` が指定された `Kany8sCluster` で kro instance が作成される

- [ ] kro instance の spec 反映ルールを固定する（idempotent）
  - Touch: `internal/controller/infrastructure/kany8scluster_controller.go`
  - MVP 既定:
    - `Kany8sCluster.spec.kroSpec` を instance `.spec` に展開
    - `.spec.clusterName` / `.spec.clusterNamespace` は常に `Kany8sCluster` から注入
  - DoD: instance `.spec` の手動変更が reconcile で戻る

- [ ] status/conditions を揃える（provisioned/Ready/failure）
  - Touch: `internal/controller/infrastructure/kany8scluster_controller.go`
  - 入力: `internal/kro/status.go` の `status.ready/reason/message`
  - DoD:
    - `status.initialization.provisioned = (instance.status.ready == true)`
    - `Ready` Condition は `provisioned` と同じ意味
    - `failureReason/failureMessage` は terminal error のみ（待機中はクリア）

### 14.4 Tests

- [ ] fake client unit test で stub/kro mode の最小フローを固定する
  - Touch: `internal/controller/infrastructure/kany8scluster_reconciler_test.go`
  - ケース例:
    - stub mode: `provisioned=true` + `Ready=True`
    - kro mode: instance 未作成 -> 作成される
    - kro mode: instance `status.ready=true` -> `provisioned=true`
    - kro mode: status 欠落/false -> `provisioned=false` で待機できる
    - kro mode: RGD が無い/不正 -> Condition に理由が出て待機/失敗できる
  - Run: `make test`

- [ ] devtools テストを追加/更新する（API/CRD/samples の回帰検知）
  - Touch: `internal/devtools/kany8scluster_api_test.go`, `internal/devtools/crd_bases_test.go` など
  - DoD: `make test` で CRD 生成・samples の整合が壊れたら検知できる

### 14.5 Examples

- [ ] infra 向けの最小 RGD 例を追加し、`Kany8sCluster` と組み合わせた例を用意する
  - Add: `examples/kro/infra/` (RGD yaml)
  - Update (or add): `examples/capi/` (Cluster + infraRef の例)
  - DoD: `kubectl apply` で `Kany8sCluster` が kro mode で Provisioned になるデモができる
