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

## M1: Kany8sControlPlane MVP (CRD + Controller)

### リポジトリ/開発環境の立ち上げ
- [ ] Go module パスを決めて `go mod init <module>` を実行する (例: `github.com/<org>/kany8s`)
- [ ] Go バージョンを決めて `go.mod` の `go` directive を固定する
- [ ] Kubebuilder で scaffold を生成する: `kubebuilder init --domain cluster.x-k8s.io --repo <module>`
- [ ] `make test` / `make generate` / `make manifests` / `make run` がローカルで通ることを確認する
- [ ] `hack/tools.go` 等で `controller-gen` 等のツールバージョンを pin する
- [ ] `Dockerfile` / `Makefile` の `docker-build` / `docker-push` ターゲットを整備する

### API: Kany8sControlPlane CRD を定義する
- [ ] API を生成する: `kubebuilder create api --group controlplane --version v1alpha1 --kind Kany8sControlPlane --resource --controller=false`
- [ ] `api/v1alpha1/kany8scontrolplane_types.go` に `spec.version` (required) を追加する
- [ ] `api/v1alpha1/kany8scontrolplane_types.go` に `spec.resourceGraphDefinitionRef.name` (required) を追加する
- [ ] `api/v1alpha1/kany8scontrolplane_types.go` に `spec.kroSpec` (optional; arbitrary object) を追加する (`runtime.RawExtension` or `apiextensionsv1.JSON` を採用)
- [ ] `api/v1alpha1/kany8scontrolplane_types.go` に `spec.controlPlaneEndpoint` (optional; `clusterv1.APIEndpoint`) を追加する (controller が設定)
- [ ] `api/v1alpha1/kany8scontrolplane_types.go` に `status.initialization.controlPlaneInitialized` を追加する
- [ ] `api/v1alpha1/kany8scontrolplane_types.go` に `status.conditions` を追加し、`GetConditions/SetConditions` を実装する
- [ ] `api/v1alpha1/kany8scontrolplane_types.go` に `status.failureReason` / `status.failureMessage` を追加する
- [ ] `make generate` / `make manifests` を実行し `config/crd/bases/` が更新されることを確認する

### Controller: RGD 参照と kro instance の作成/更新
- [ ] controller を生成する: `kubebuilder create api --group controlplane --version v1alpha1 --kind Kany8sControlPlane --controller --resource=false`
- [ ] `controllers/kany8scontrolplane_controller.go` で `spec.resourceGraphDefinitionRef.name` から `kro.run/v1alpha1 ResourceGraphDefinition` を取得できるようにする
- [ ] `ResourceGraphDefinition.spec.schema.kind` を読み、kro instance の `kind` に使う
- [ ] `ResourceGraphDefinition.spec.schema.apiVersion` を読み、kro instance の `apiVersion` を組み立てる (例: `kro.run/<schema.apiVersion>`; 既に `/` を含む場合はそのまま)
- [ ] kro instance を `unstructured.Unstructured` で扱い、`metadata.name/namespace` を `Kany8sControlPlane` と一致させる
- [ ] kro instance の `spec` を `spec.kroSpec` から構築し、`spec.version` は必ず `Kany8sControlPlane.spec.version` で上書きする
- [ ] kro instance に `OwnerReference`(controller=true) を付与し、削除連鎖できるようにする
- [ ] `controllerutil.CreateOrUpdate` または Server-Side Apply のどちらを採用するか決め、idempotent に spec を反映できるようにする
- [ ] RGD が見つからない/不正な場合のエラーを Condition と Event に出し、適切に requeue する

### Controller: status 正規化 contract の消費と CAPI contract の充足
- [ ] kro instance の `status.endpoint` を読み取る (string)
- [ ] kro instance の `status.ready` を読み取る (bool; 取得できない場合は false 扱い)
- [ ] endpoint parse ユーティリティを追加する: `internal/endpoint/parse.go` (入力: `https://host[:port]` or `host[:port]`; port 省略は 443)
- [ ] endpoint が parse できたら `Kany8sControlPlane.spec.controlPlaneEndpoint.host/port` を設定する
- [ ] endpoint が確定したら `Kany8sControlPlane.status.initialization.controlPlaneInitialized=true` を設定する
- [ ] kro instance の `status.reason` / `status.message` があれば `failureReason/failureMessage` と Condition の Reason/Message に反映する
- [ ] Condition の状態遷移を定義する (例: Creating/Ready/Failed) と、`sigs.k8s.io/cluster-api/util/conditions` で一貫して更新する
- [ ] kro instance が未 Ready の間は `RequeueAfter` でポーリングする間隔(例: 10-30s)を固定する

### Controller: 動的 GVK の watch 戦略
- [ ] MVP は `RequeueAfter` ポーリングで進め、kro instance の status 反映が動くことを最優先で確認する
- [ ] 拡張として dynamic informer(`dynamicinformer.NewFilteredDynamicSharedInformerFactory`)で GVR ごとに watch を立てる設計にするか判断し、採用する場合は `internal/dynamicwatch/` を追加する
- [ ] dynamic watch を採用する場合、kro instance の OwnerReference から `Kany8sControlPlane` を特定して reconcile queue へ enqueue する

### RBAC/配布 (最低限)
- [ ] `+kubebuilder:rbac` を追加し、`kany8scontrolplanes` の get/list/watch/create/update/patch/delete と status/finalizers を許可する
- [ ] `+kubebuilder:rbac` を追加し、`resourcegraphdefinitions.kro.run` の get/list/watch を許可する
- [ ] kro instance を作成/更新するため、`kro.run` group の生成 CR に対する create/get/list/watch/update/patch を許可する(最小権限の設計は後続で詰める)
- [ ] `make manifests` で RBAC が生成されることを確認する

### テスト
- [ ] endpoint parse の table-driven unit test を追加する: `internal/endpoint/parse_test.go`
- [ ] RGD schema(apiVersion/kind) から instance GVK を解決する unit test を追加する: `internal/kro/gvk_test.go`
- [ ] fake client で controller の "未 Ready -> requeue" と "Ready+endpoint -> spec/status 更新" を検証する unit test を追加する
- [ ] `make test` が CI で実行できるようにする (envtest を使う場合は setup-envtest の導入も含める)

### サンプル/ドキュメント
- [ ] `examples/capi/cluster.yaml` に `Cluster` + `Kany8sControlPlane` の最小例を追加する
- [ ] `examples/kro/` に "ready/endpoint" 正規化 contract を満たす最小 RGD の例を追加する
- [ ] `README.md` に "install -> apply RGD -> apply Cluster" の手順を追記する

## M2: AWS/EKS 参照 RGD (ACK) と end-to-end 動作確認

### RGD: eks-control-plane
- [ ] `examples/kro/eks/eks-control-plane-rgd.yaml` を作成し、ACK EKS Cluster + 前提 IAM Role を graph に含める
- [ ] RGD の instance `status.endpoint` を `${cluster.status.endpoint}` で射影する
- [ ] RGD の instance `status.ready` を `${int(cluster.status.status == "ACTIVE" && cluster.status.endpoint != "") == 1}` 等で安定して materialize される形にする(kro v0.7.1 の bool 欠落回避)
- [ ] Role -> Cluster の依存を `${clusterRole.status.ackResourceMetadata.arn}` 参照で DAG 化し、ACK の race/Terminal を避ける
- [ ] `readyWhen` は self resource のみ参照できる前提で、Cluster resource 自身の readyWhen に ACTIVE/endpoint 条件を置く

### 部品化/合成 (任意)
- [ ] `examples/kro/eks/eks-addons-rgd.yaml` を作成し、Addon 群を別 RGD に分離する
- [ ] `examples/kro/eks/pod-identity-set-rgd.yaml` を作成し、Role 群 + PodIdentityAssociation 群を別 RGD に分離する
- [ ] `examples/kro/eks/platform-cluster-rgd.yaml` を作成し、chaining で部品 RGD を束ねる(親 status に ready/endpoint を統一)

### 動作確認手順の整備
- [ ] kind (管理クラスタ) + kro のセットアップ手順を `docs/runbooks/kind-kro.md` にまとめる
- [ ] ACK コントローラ導入/認証の前提を `docs/runbooks/ack.md` にまとめる
- [ ] `Cluster` 適用から endpoint/initialized が立つまでの観測コマンド集を `docs/runbooks/e2e.md` にまとめる

## M3: kubeconfig Secret (CAPI contract)

### kubeconfig 生成方式の決定と実装
- [ ] provider-agnostic に kubeconfig を得る contract を決める(例: kro instance status に `kubeconfigSecretRef` を追加する)
- [ ] RGD 側で kubeconfig Secret を作る場合、Secret 名/namespace/labels/type を CAPI contract に合わせる設計にする
- [ ] Kany8s 側で kubeconfig Secret を作る場合、入力(認証/CA/endpoint)をどこから得るかを定義し実装する
- [ ] `<cluster>-kubeconfig` Secret の `type=cluster.x-k8s.io/secret` と `cluster.x-k8s.io/cluster-name` label を満たす
- [ ] kubeconfig Secret の作成/更新を controller の reconcile に組み込み、回帰テストを追加する

## M4: ClusterClass/Topology と Template API

### Template CRD
- [ ] `Kany8sControlPlaneTemplate` / `Kany8sClusterTemplate` の API を追加する (ClusterClass から参照できる形)
- [ ] `Cluster.spec.topology.version` を `Kany8sControlPlane.spec.version` へ流し込む前提で設計する
- [ ] variables/patches で `kroSpec` にマップする方針を決め、サンプル `ClusterClass` を `examples/capi/clusterclass.yaml` に追加する

### Topology 動作確認
- [ ] `clusterctl` で Kany8s provider を扱える packaging 方針を決める(components.yaml 生成など)
- [ ] ClusterClass 経由で `Cluster` を作成し、Kany8sControlPlane が生成/更新されることを確認する

## M5: マルチプロバイダ拡張と配布

### 配布/リリース
- [ ] `config/default` を整備し `make deploy` でインストールできるようにする
- [ ] Helm chart を作るか、clusterctl provider として `components.yaml` を提供するかを決める
- [ ] バージョニングとリリースフロー(タグ/リリースノート/イメージ公開)を `docs/release.md` にまとめる

### Provider/RGD カタログ
- [ ] `docs/rgd-contract.md` を作成し、Kany8s が期待する正規化 status(ready/endpoint/reason/message)を明文化する
- [ ] GKE/AKS 向けの "ControlPlane RGD 雛形" を `examples/kro/` に追加する(実装はスタブでも良い)
- [ ] RGD の static analysis/kro 既知問題(NetworkPolicy 等)の注意点を `docs/rgd-guidelines.md` にまとめる

### Kany8sCluster (Infrastructure provider) の最小実装 (任意)
- [ ] `Kany8sCluster` CRD を追加し、`Cluster.spec.infrastructureRef` を満たす最小 contract を定義する
- [ ] `Kany8sCluster` controller で Ready 条件を立て、CAPI の `InfrastructureReady` を unblock できるようにする
