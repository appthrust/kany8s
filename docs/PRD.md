# PRD: Kany8s

- 作成日: 2026-01-25
- ステータス: Draft (design-first / prototype)

Kany8s は Cluster API (CAPI) と kro (ResourceGraphDefinition / RGD) を組み合わせ、あらゆるクラウドの managed Kubernetes control plane を "kubernetes-native" に作成・運用するための Cluster API provider suite です。

本 PRD は、Why/What/How を 1 枚にまとめた形で記載します。

## 1. 概要

### 製品概要

- プロダクト名: Kany8s (k(ro)+any+k8s)
- タグライン: Any k8s, powered by kro.
- 位置付け: Cluster API provider suite
  - `Kany8sControlPlane` (ControlPlane provider)
  - `Kany8sCluster` (Infrastructure provider / planned)

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
- 小さく分割・合成: "巨大な 1 枚 RGD" を避け、部品 RGD + chaining で再利用可能にする。
- Ready の定義を守る: ControlPlane の Ready は "API endpoint を設定できる" を意味し、Addon 等の周辺リソースの ready とは分離する。
- Secrets は最小限: kubeconfig Secret 等、CAPI contract 上必須なもの以外は "汎用 outputs" を原則持ち込まない。

## 4. スコープ

### MVP (最小実用)

- `Kany8sControlPlane` CRD + controller
  - `spec.resourceGraphDefinitionRef` で RGD を選択
  - RGD の生成 GVK を解決し、kro instance を 1:1 で作成/更新
  - kro instance の `status.ready` / `status.endpoint` を監視し、CAPI contract に従い以下を更新
    - `Kany8sControlPlane.spec.controlPlaneEndpoint` (host/port)
    - `Kany8sControlPlane.status.initialization.controlPlaneInitialized`
    - `Kany8sControlPlane.status.conditions`
- 参照実装として AWS/EKS の ControlPlane RGD を提供 (ACK ベース)
  - ControlPlane とその前提(例: 必須 IAM Role)までを責務とする
  - status 正規化: `ready/endpoint/(reason/message)` を提供
- ドキュメント/サンプル
  - CAPI `Cluster` 直指定、および `ClusterClass`(planned) の利用イメージ

### スコープ外 (現時点でやらない)

- CAPT の Template→Apply パターンを中核概念として再現しない
- Terraform の outputs のような "汎用 outputs → Secret" をコア設計として採用しない
- ControlPlane Ready に Addon/S3/SQS/EventBridge 等の周辺 ready を含めない(必要なら別 RGD へ)
- Worker(Machine/MachinePool) の作成・管理
- マルチテナント機能(05-multi-tenant 的な RGD 例をそのまま Kany8s のコア要件にはしない)

### 未決 / Planned

- `Kany8sCluster` (Infrastructure provider) の扱い
  - 最小の InfrastructureRef を満たす形から開始し、共有前提(VPC 等)を持つかは段階的に判断
- kubeconfig Secret 管理 (CAPI contract 上必須)
  - provider-agnostic に実現するため、RGD 側で必要情報(例: CA data 等)を正規化して渡す方式を検討

## 5. 対象ユーザー

- Platform Engineer / SRE: CAPI を採用した管理クラスタ上で、managed control plane をセルフサービス化したい
- 組織内の "クラスタ提供者": マルチクラウド(将来)を見据え、provider 追加を "コードではなく RGD" で行いたい
- CAPI ユーザー: 既存の CAPI ワークフロー(clusterctl / GitOps / ClusterClass)に managed Kubernetes を統合したい

## 6. ユースケース

- UC1: CAPI `Cluster` から managed control plane を作る
  - `Cluster.spec.controlPlaneRef` に `Kany8sControlPlane` を指定し、RGD を参照して EKS 等を作成する
- UC2: ClusterClass/Topology による標準化/セルフサービス (planned)
  - `Cluster.spec.topology.version` を単一ソースとして、kro instance `spec.version` へ注入する
  - 変数(patches/variables)で `kroSpec` を供給し、環境ごとの差分(リージョン/VPC 等)を吸収する
- UC3: provider 追加/切り替え
  - controller は変更せず、RGD を追加(例: `gke-control-plane`)して `resourceGraphDefinitionRef` を切り替える
- UC4: 周辺リソースの合成 (非MVP)
  - Addons/PodIdentity/S3/SQS/EventBridge 等は部品 RGD として提供し、必要に応じて chaining で platform を構成する

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
    - `status.failureReason` / `status.failureMessage` (可能なら kro status.reason/message を反映)

- Should: `Kany8sCluster` CRD (InfrastructureRef を満たす最小実装)
- Could: `Kany8sControlPlaneTemplate` / `Kany8sClusterTemplate` (ClusterClass 利用)

### 9.2 Controller 振る舞い

- Must: `Kany8sControlPlane` reconcile
  - 参照 RGD を取得し、生成される instance の GVK を解決できる
  - kro instance を 1:1 で作成/更新できる (OwnerReference 付与)
  - kro instance へ `spec.version` を必ず注入(上書き)できる
  - kro instance の `status.ready/endpoint` を読み、CAPI contract に従って endpoint/initialized/conditions を更新できる
  - delete 時は OwnerReference により kro instance を削除連鎖できる

- Should: 状態/失敗理由の可観測性
  - kro instance の `status.reason/message` を Condition の Reason/Message に反映
  - K8s Event として重要状態変化を出す

- Could: 安全な drift/変更検知
  - `kroSpec` の差分があれば kro instance を更新
  - `resourceGraphDefinitionRef` 切り替え時の migration ポリシー(明示的な break-glass)

### 9.3 kro instance status 正規化 contract

- Must: `status.ready: boolean` (ControlPlane ready)
- Must: `status.endpoint: string` (`https://host[:port]` or `host[:port]`)
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
apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
metadata:
  name: demo-cluster
spec:
  infrastructureRef:
    apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
    kind: Kany8sCluster
    name: demo-cluster
  controlPlaneRef:
    apiVersion: controlplane.cluster.x-k8s.io/v1alpha1
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
- CAPI v1beta1 `Cluster` と ControlPlane provider contract に準拠する
- RGD は cluster-scoped 前提、kro instance は `Kany8sControlPlane` と同一 namespace に作成する

### 11.2 kro 実装制約への適合 (kro v0.7.1 検証より)

- `spec.schema.status` の CEL では `schema.*` を参照できない
  - status は resource id 変数から射影する設計とする
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

- M0: PRD/設計確定 (現在)
- M1: `Kany8sControlPlane` CRD + controller の骨格
  - kro instance の作成/更新
  - endpoint/initialized/conditions の反映
- M2: AWS/EKS 参照 RGD(ACK) の提供とドキュメント
  - status 正規化(ready/endpoint)
  - RGD 分割方針(部品 RGD + chaining)のサンプル
- M3: kubeconfig Secret 要件の整理と実装方針決定
  - provider-agnostic な contract 拡張(例: CA data/secretRef)の検討
- M4: ClusterClass/Topology 経由の利用サンプル
  - Template CRD
  - variables/patches -> kroSpec の設計
- M5: 追加 provider(RGD) カタログ拡張 (AKS/GKE など)

# TODO

このセクションは「実装の作業指示書」に近い TODO リストです。

方針:

- 1つのチェックボックス = だいたい1コミット(目安)
- 各項目に「成果物(触るファイル/追加するもの)」と「完了条件(DoD)」を書く
- 選択肢がある場合は「MVP の既定」を TODO 内に明記し、後で見直せる形にする

## M1: Kany8sControlPlane MVP (CRD + Controller)

### リポジトリ/開発環境の立ち上げ
- [x] Go module + kubebuilder scaffold を生成する
  - コマンド: `go mod init <module>` / `kubebuilder init --domain cluster.x-k8s.io --repo <module>`
  - 成果物: `go.mod`, `main.go`, `config/`, `Makefile`
  - DoD: `make help` が表示でき、`go test ./...` が通る
- [x] Go バージョンと開発ツールバージョンを pin する
  - 触る: `go.mod` の `go` directive, `hack/tools.go` (例: `controller-gen`, `kustomize`, `setup-envtest`)
  - DoD: クリーンな checkout から `make generate` が再現性を持って動く
- [x] ローカルの開発ループを確認し、前提/手順を README に残す
  - 実行: `make test` / `make generate` / `make manifests` / `make run`
  - 成果物: `README.md` に「必要ツール」「最短の動かし方」が書かれている
  - DoD: 別環境(=新しい作業者)が README の手順だけでローカル実行できる
- [x] Controller イメージの build/push ターゲットを整備する
  - 触る: `Dockerfile`, `Makefile` (`docker-build`, `docker-push`, `IMG ?= ...`)
  - DoD: `make docker-build IMG=example.com/kany8s/controller:dev` が成功する
- [x] (推奨) CI で `make test` / `make manifests` が回るようにする
  - 追加: `.github/workflows/ci.yaml`
  - DoD: PR/Push で `make test` が実行され、失敗が検知できる

### API: Kany8sControlPlane CRD を定義する
- [x] API scaffold を生成する
  - コマンド: `kubebuilder create api --group controlplane --version v1alpha1 --kind Kany8sControlPlane --resource --controller=false`
  - 成果物: `api/v1alpha1/kany8scontrolplane_types.go`
  - DoD: `make generate` が通る(フィールド追加前でも OK)
- [x] `Kany8sControlPlaneSpec` を MVP 要件どおりに定義する
  - 触る: `api/v1alpha1/kany8scontrolplane_types.go`
  - 追加:
    - `spec.version` (required)
    - `spec.resourceGraphDefinitionRef.name` (required)
    - `spec.kroSpec` (optional; arbitrary object)
      - MVP 既定: `apiextensionsv1.JSON` を採用し unknown fields を許容する
    - `spec.controlPlaneEndpoint` (optional; `clusterv1.APIEndpoint`; controller が設定)
  - DoD: `make generate` 後に `spec.version` と `spec.resourceGraphDefinitionRef.name` が CRD 上 required になる
- [x] `Kany8sControlPlaneStatus` を MVP 要件どおりに定義する
  - 触る: `api/v1alpha1/kany8scontrolplane_types.go`
  - 追加:
    - `status.initialization.controlPlaneInitialized`
    - `status.conditions` + `GetConditions/SetConditions` (Cluster API util/conditions と互換)
    - `status.failureReason` / `status.failureMessage`
  - DoD: `make test` が通り、`make manifests` の CRD に status subresource が生成される
- [x] (任意/推奨) `kubectl get kany8scontrolplanes` が読みやすくなるよう PrintColumns を追加する
  - 触る: `api/v1alpha1/kany8scontrolplane_types.go`
  - 例: INITIALIZED/ENDPOINT をそれぞれ `.status.initialization.controlPlaneInitialized` / `.spec.controlPlaneEndpoint.host` から表示
  - DoD: `make manifests` 後の CRD に additionalPrinterColumns が入る
- [x] `make generate` / `make manifests` を実行し `config/crd/bases/` の更新を確認する
  - DoD: `config/crd/bases/` の差分が API 変更を反映している

### Controller: RGD 参照と kro instance の作成/更新
- [x] controller scaffold を生成し、manager に登録される状態にする
  - コマンド: `kubebuilder create api --group controlplane --version v1alpha1 --kind Kany8sControlPlane --controller --resource=false`
  - 成果物: `controllers/kany8scontrolplane_controller.go`
  - DoD: `make test` が通る(ロジック追加前でも OK)
- [x] `ResourceGraphDefinition` の取得と「生成される instance GVK」の解決ロジックを実装する
  - 触る/追加:
    - 追加: `internal/kro/gvk.go` (例: `ResolveInstanceGVK(ctx, client, rgdName) (schema.GroupVersionKind, error)`)
    - 触る: `controllers/kany8scontrolplane_controller.go`
  - 実装メモ:
    - RGD 自体は `kro.run/v1alpha1` / `kind=ResourceGraphDefinition`
    - instance の GVK は `rgd.spec.schema.apiVersion` と `rgd.spec.schema.kind` から作る
      - `schema.apiVersion` が `v1alpha1` のように group を含まない場合は `kro.run/<schema.apiVersion>` にする
  - DoD: `internal/kro/gvk_test.go` で table-driven に期待 GVK が解決できる
- [x] kro instance を `unstructured.Unstructured` として 1:1 で create/update できるようにする
  - 触る: `controllers/kany8scontrolplane_controller.go`
  - MVP 既定:
    - kro instance/RGD ともに `unstructured.Unstructured` で扱い、kro の Go API 依存を持ち込まない
    - patch 方式は `controllerutil.CreateOrUpdate` を採用する(SSA は後で検討)
  - DoD: `metadata.name/namespace` が `Kany8sControlPlane` と一致した instance が作成/更新される
- [x] kro instance `spec` の構築ルールを固定し、idempotent に反映する
  - 仕様:
    - `Kany8sControlPlane.spec.kroSpec` を instance `.spec` に展開する
    - `.spec.version` は必ず `Kany8sControlPlane.spec.version` で上書きする
  - DoD: `spec.version` を手動で変えても reconcile で元に戻る
- [x] kro instance に `OwnerReference`(controller=true) を付与し、削除連鎖できるようにする
	- DoD: `Kany8sControlPlane` 削除で kro instance が GC される
- [x] RGD が見つからない/不正な場合の扱いを「Condition + Event + requeue」で統一する
  - DoD: `kubectl describe kany8scontrolplane <name>` で失敗理由が追える

### Controller: status 正規化 contract の消費と CAPI contract の充足
- [x] kro instance status (`ready/endpoint/reason/message`) を安全に読むヘルパーを追加する
  - 追加: `internal/kro/status.go`
  - DoD: status field が欠落していても panic せず、(ready=false, endpoint="") として扱える
- [x] endpoint parse ユーティリティを追加する
  - 追加: `internal/endpoint/parse.go`
  - 仕様: 入力は `https://host[:port]` または `host[:port]`。port 省略は 443。
  - DoD: `internal/endpoint/parse_test.go` の table-driven test が通る
- [x] endpoint を `Kany8sControlPlane.spec.controlPlaneEndpoint` (host/port) に反映する
  - 触る: `controllers/kany8scontrolplane_controller.go`
  - DoD: endpoint が parse できたら `spec.controlPlaneEndpoint` が埋まる
- [x] endpoint が確定したら `status.initialization.controlPlaneInitialized=true` を設定する
  - DoD: initialized が True になった後は false に戻らない(仕様として戻す必要が無い)
- [x] `failureReason/failureMessage` と Conditions を `ready/endpoint/reason/message` に基づいて更新する
  - 触る: `controllers/kany8scontrolplane_controller.go`
  - 条件(例): Creating/Ready/Failed を `sigs.k8s.io/cluster-api/util/conditions` で更新
  - DoD: Ready=false の間は Creating が立ち、Ready=true + endpoint で Ready が True になる
- [x] 未 Ready の間のポーリング間隔を定数化する
  - 例: `internal/constants/constants.go` などに `RequeueAfter = 15 * time.Second`
  - DoD: reconcile が過剰に回らず、endpoint/ready の変化に追従できる

### Controller: 動的 GVK の watch 戦略
- [x] MVP は `RequeueAfter` ポーリングで進め、まず「status 反映が動く」ことを確認する
  - DoD: kro instance の endpoint/ready が変わると、次回 reconcile で ControlPlane 側が追従する
- [x] (拡張) dynamic watch の要否を判断し、採用する場合は実装する
  - 判断基準(例): 反応速度が課題/クラスタ数が多くポーリングが重い/instance の GVK が少数に収まる
  - 採用する場合の成果物: `internal/dynamicwatch/` + `dynamicinformer.NewFilteredDynamicSharedInformerFactory`
  - DoD: kro instance の update で該当 `Kany8sControlPlane` が enqueue される

### RBAC/配布 (最低限)
- [x] `+kubebuilder:rbac` を追加し、ControlPlane CRD 自身の権限を揃える
  - 触る: `controllers/kany8scontrolplane_controller.go`
  - DoD: `kany8scontrolplanes` の CRUD + status/finalizers が生成 RBAC に含まれる
- [x] `ResourceGraphDefinition` を読む RBAC を追加する
  - DoD: `resourcegraphdefinitions.kro.run` の get/list/watch が生成 RBAC に含まれる
- [x] 動的に生成される kro instance を create/update できる RBAC を追加する
  - 注意: GVK が動的なので、MVP は `kro.run` group を広めに許可する(最小権限化は後続)
  - DoD: `kro.run` group の create/get/list/watch/update/patch が生成 RBAC に含まれる
- [x] Event を出す場合は events の RBAC を追加する
  - DoD: controller が `events.k8s.io` / `corev1` event を作成できる
- [x] `make manifests` で RBAC が生成されることを確認する

### テスト
- [x] endpoint parse の table-driven unit test を追加する
  - 追加: `internal/endpoint/parse_test.go`
  - DoD: `make test` で parse の境界値(hostのみ/host:port/https URL/不正入力)をカバーできる
- [x] RGD schema(apiVersion/kind) -> instance GVK 解決の unit test を追加する
  - 追加: `internal/kro/gvk_test.go`
  - DoD: `schema.apiVersion` が `v1alpha1` / `example.com/v1alpha1` の両方で期待結果になる
- [ ] fake client で controller の reconcile を unit test する
  - 追加: `controllers/kany8scontrolplane_controller_test.go` (例)
  - シナリオ例:
    - instance 未 Ready -> `RequeueAfter` が返る
    - Ready + endpoint -> `spec.controlPlaneEndpoint` と `status.initialization.controlPlaneInitialized` が更新される
  - DoD: watch を使わず reconcile 単体の期待がテストで固定できる
- [ ] `make test` を CI で実行できるようにする
  - 前提: envtest を使う場合は `setup-envtest` を tools として pin する
  - DoD: CI 上で `make test` が動作し、失敗が検知できる

### サンプル/ドキュメント
- [x] `examples/capi/cluster.yaml` に `Cluster` + `Kany8sControlPlane` の最小例を追加する
  - DoD: 例の YAML だけで「どの CR を apply するか」が理解できる
- [x] `examples/kro/` に "ready/endpoint" 正規化 contract を満たす最小 RGD の例を追加する
  - DoD: RGD instance の `status.ready/endpoint` が必ず出力される(欠落しない)例になっている
- [ ] `README.md` に "install -> apply RGD -> apply Cluster" の手順を追記する
  - DoD: kind 上での最短手順が 1 セクションで追える

## M2: AWS/EKS 参照 RGD (ACK) と end-to-end 動作確認

### RGD: eks-control-plane
- [ ] `examples/kro/eks/eks-control-plane-rgd.yaml` を作成し、ACK EKS Cluster + 前提 IAM Role を graph に含める
  - DoD: `kubectl apply -f` で RGD が `ResourceGraphAccepted=True` になる
- [ ] RGD instance の `status.endpoint` を `${cluster.status.endpoint}` で射影する
  - 注意: kro v0.7.1 の "文字列テンプレート" の落とし穴があるため、必要なら CEL 1式で連結する (`docs/kro.md`)
  - DoD: endpoint が欠落せず、常に string として出力される
- [ ] RGD instance の `status.ready` を "欠落しにくい" 形で materialize する
  - 例: `${int(cluster.status.status == "ACTIVE" && cluster.status.endpoint != "") == 1}` (kro v0.7.1 の bool 欠落回避)
  - DoD: ready が常に boolean として出力される
- [ ] Role -> Cluster の依存を `${clusterRole.status.ackResourceMetadata.arn}` 参照で DAG 化する
  - DoD: Role 未作成の race で ACK Terminal に落ちる確率を下げられる
- [ ] `readyWhen` は self resource のみ参照できる前提で、Cluster resource 自身の readyWhen に判定を置く
  - DoD: Cluster の `readyWhen` が `ACTIVE` + `endpoint != ""` を待つ

### 部品化/合成 (任意)
- [ ] `examples/kro/eks/eks-addons-rgd.yaml` を作成し、Addon 群を別 RGD に分離する
  - DoD: Addon の Ready/依存が ControlPlane Ready と分離できる
- [ ] `examples/kro/eks/pod-identity-set-rgd.yaml` を作成し、Role 群 + PodIdentityAssociation 群を別 RGD に分離する
  - DoD: Role -> PodIdentityAssociation の順序が DAG で保証できる
- [ ] `examples/kro/eks/platform-cluster-rgd.yaml` を作成し、chaining で部品 RGD を束ねる
  - DoD: 親 instance の `status.ready/endpoint` が ControlPlane と一致する

### 動作確認手順の整備
- [ ] kind (管理クラスタ) + kro のセットアップ手順を `docs/runbooks/kind-kro.md` にまとめる
  - DoD: "再現環境を作る" 手順がコピペで実行できる
- [ ] ACK コントローラ導入/認証の前提を `docs/runbooks/ack.md` にまとめる
  - DoD: 最低限 "何をインストールし、どの認証情報が必要か" が明記されている
- [ ] `Cluster` 適用から endpoint/initialized が立つまでの観測コマンド集を `docs/runbooks/e2e.md` にまとめる
  - DoD: "詰まった時にどこを見るか" が一覧できる

## M3: kubeconfig Secret (CAPI contract)

### kubeconfig 生成方式の決定と実装
- [ ] provider-agnostic に kubeconfig を得る contract を決め、`docs/design.md` に追記する
  - 例: kro instance status に `kubeconfigSecretRef` (name/namespace) を追加する
  - DoD: "RGD 側で何を出すか / Kany8s 側で何を読むか" が 1 枚で説明できる
- [ ] (方針 A) RGD 側で kubeconfig Secret を作る場合の要件を定義する
  - DoD: Secret 名/namespace/labels/type が CAPI contract と一致する
- [ ] (方針 B) Kany8s 側で kubeconfig Secret を作る場合の入力 contract を定義する
  - DoD: endpoint/CA/token 等の入手元が矛盾なく決まっている
- [ ] `<cluster>-kubeconfig` Secret の contract を満たす
  - `type=cluster.x-k8s.io/secret`
  - `cluster.x-k8s.io/cluster-name=<cluster>` label
  - DoD: Cluster API が kubeconfig Secret を発見できる
- [ ] kubeconfig Secret の作成/更新を reconcile に組み込み、回帰テストを追加する
  - DoD: kubeconfig 周りの変更がテストで検知できる

## M4: ClusterClass/Topology と Template API

### Template CRD
- [ ] `Kany8sControlPlaneTemplate` / `Kany8sClusterTemplate` の API を追加する(ClusterClass から参照できる形)
  - DoD: ClusterClass から参照できる `Template` が作れる
- [ ] `Cluster.spec.topology.version` -> `Kany8sControlPlane.spec.version` の流し込み方針を設計する
  - DoD: version の single source of truth が `Cluster.spec.topology.version` になる
- [ ] variables/patches -> `kroSpec` マッピング方針を決め、サンプル `ClusterClass` を追加する
  - 追加: `examples/capi/clusterclass.yaml`
  - DoD: "どの variables が kroSpec のどこに入るか" が例で追える

### Topology 動作確認
- [ ] `clusterctl` で Kany8s provider を扱える packaging 方針を決める
  - 例: `components.yaml` 生成、Helm chart の採否
  - DoD: "clusterctl init" 相当の手順が 1 つに定まる
- [ ] ClusterClass 経由で `Cluster` を作成し、Kany8sControlPlane が生成/更新されることを確認する
  - DoD: topology 変更で kro instance まで追従する

## M5: マルチプロバイダ拡張と配布

### 配布/リリース
- [ ] `config/default` を整備し `make deploy` でインストールできるようにする
  - DoD: kind などの検証クラスタに 1 コマンドでデプロイできる
- [ ] Helm chart を作るか、clusterctl provider として `components.yaml` を提供するかを決める
  - DoD: "利用者がどうインストールするか" が 1 つに決まる
- [ ] バージョニングとリリースフロー(タグ/リリースノート/イメージ公開)を `docs/release.md` にまとめる
  - DoD: リリース作業が手順書どおりに実行できる

### Provider/RGD カタログ
- [ ] `docs/rgd-contract.md` を作成し、Kany8s が期待する正規化 status(ready/endpoint/reason/message)を明文化する
  - DoD: provider 実装者が "どの status を出せば良いか" を迷わない
- [ ] GKE/AKS 向けの "ControlPlane RGD 雛形" を `examples/kro/` に追加する(スタブでも可)
  - DoD: 新しい provider を追加する最小テンプレがある
- [ ] RGD の static analysis/kro 既知問題(NetworkPolicy 等)の注意点を `docs/rgd-guidelines.md` にまとめる
  - 参考: `docs/kro.md` の検証結果
  - DoD: RGD 作成者がハマりやすい点を事前に回避できる

### Kany8sCluster (Infrastructure provider) の最小実装 (任意)
- [ ] `Kany8sCluster` CRD を追加し、`Cluster.spec.infrastructureRef` を満たす最小 contract を定義する
  - DoD: `Cluster` の infraRef が "とりあえず" 解決できる
- [ ] `Kany8sCluster` controller で Ready 条件を立て、CAPI の `InfrastructureReady` を unblock できるようにする
  - DoD: InfrastructureReady が True になり、ControlPlane 側のフローへ進める
