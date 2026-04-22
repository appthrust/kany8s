# EKS kubeconfig rotator plugin design (CAPI RemoteConnectionProbe)

- 作成日: 2026-02-07
- ステータス: Proposed

## 背景

Cluster API (CAPI) は Workload Cluster へ接続するために、namespace 内の `<cluster>-kubeconfig` Secret を参照します。

- Secret 名: `<cluster>-kubeconfig`
- `type`: `cluster.x-k8s.io/secret`
- `labels["cluster.x-k8s.io/cluster-name"] = <cluster>`
- kubeconfig 本体: `data.value`

そして CAPI v1.12 系では `RemoteConnectionProbe` が Cluster の可用性計算に含まれるため、Workload Cluster の API に到達できない（もしくは kubeconfig が機能しない）場合、`Cluster Available=True` まで到達しません。

EKS の kubeconfig は一般に `exec` で `aws eks get-token` を実行して IAM token を取得します。
しかし CAPI controller (management cluster 側) のコンテナ環境に `aws` バイナリや AWS 認証情報が無いことが多く、そのままだと `RemoteConnectionProbe` が成立しません。

さらに IAM token は短命（おおむね 15 分）なので、token を kubeconfig に埋め込む場合は定期更新が必要です。

## ACK(EKS) が提供する情報（token は出ない）

ACK の `clusters.eks.services.k8s.aws` CRD は、EKS API の `DescribeCluster` 相当の情報を `status` に反映します。
少なくとも以下のフィールドがあり、kubeconfig 生成に使えます。

- `status.endpoint`
- `status.certificateAuthority.data`

一方で **token は CRD からは提供されません**。
これは EKS 自体の API が token を返す設計ではなく、`aws eks get-token` 等がクライアント側で STS 署名付きリクエスト（GetCallerIdentity）を presign して token を作るためです。

従って、本プラグインは token を自前生成する必要があります。

本設計では、実装コスト/互換性/安全性の観点から、`aws-iam-authenticator` の token 生成ロジックを Go ライブラリとして利用します。
（AWS SDK v2 の STS presign を使い、`k8s-aws-v1.` token を生成する）

参考: `kubernetes-sigs/aws-iam-authenticator` の `pkg/token` は token の有効期限を `15m - 1m` のクッションで計算します。

## ゴール

- CAPI controller の実行環境に `aws` バイナリを要求せずに `RemoteConnectionProbe=True` を成立させる。
- `Cluster Available=True` の到達を現実的な運用構成として提供する。
- Kany8s core には EKS 依存ロジックを入れず、EKS 専用の追加コンポーネント（プラグイン）で解決する。

## Non-goals

- EKS NodeGroup / MachinePool など worker 管理を含む “フル EKS provider” 実装。
- private endpoint 等、management cluster から物理的に到達できないネットワーク要件の解決。
  - その場合 `RemoteConnectionProbe` は仕様上成立しません（到達性は別途ネットワークで担保）。

## 提案: `eks-kubeconfig-rotator` コントローラ

EKS 専用プラグインとして、management cluster に以下を追加デプロイします。

- controller 名（仮）: `eks-kubeconfig-rotator`
- 責務:
  - `Cluster` ごとに CAPI 互換 kubeconfig Secret (`<cluster>-kubeconfig`) を作成/更新する
  - kubeconfig に埋め込む IAM token を定期的に更新し、失効による `RemoteConnectionProbe` の揺れを抑える

この方式は Kany8s の kubeconfig 戦略（`docs/adr/0004-kubeconfig-secret-strategy.md`）でいう Option A に相当します。
Kany8s facade 側の `status.kubeconfigSecretRef` 連携（Option B）は使いません（理由は後述）。

## 決定事項（MVP）

- token 生成: Go 実装（`aws-iam-authenticator` の `pkg/token` を利用）
- endpoint/CA のソース: ACK EKS Cluster CR (`clusters.eks.services.k8s.aws`) を読む
- 生成する Secret:
  - CAPI probe 用: `<cluster>-kubeconfig`（token 埋め込み）
  - 人間用: `<cluster>-kubeconfig-exec`（exec kubeconfig）
- Kany8s facade との関係:
  - EKS BYO では `status.kubeconfigSecretRef` を使わず、Kany8s が `<cluster>-kubeconfig` を生成しないようにする

## 何が `Available=True` をブロックするか

`RemoteConnectionProbe` は CAPI ClusterCache が `<cluster>-kubeconfig` を読み、実際に API へ `GET /` を投げることで成立します。
よって以下のどれかが欠けると `RemoteConnectionProbe` は失敗します。

- kubeconfig Secret が存在しない
- kubeconfig が `exec` 依存で、CAPI controller 環境に実行バイナリ/認証情報が無い
- kubeconfig の bearer token が失効して Unauthorized になる（ローテーションが無い）
- endpoint が network 的に到達不能

このプラグインは上記のうち “kubeconfig の実体” と “token ローテーション” を担当します。

## インタフェース（入力・出力）

### 対象 Cluster の選別 (opt-in)

誤動作/誤課金を避けるため、デフォルトは **明示 opt-in** にします。

例:

- `Cluster.metadata.annotations["eks.kany8s.io/kubeconfig-rotator"] = "enabled"`

### EKS クラスタ識別

用語:

- `capiClusterName`: `Cluster.metadata.name`
- `eksClusterName`: EKS 上の cluster 名
- `ackClusterName`: ACK の `clusters.eks.services.k8s.aws` リソース名

デフォルト解決は Topology/ClusterClass の名前ズレを吸収する形にし、必要なら annotation で override します。

- `eksClusterName = Cluster.metadata.annotations["eks.kany8s.io/cluster-name"] ?? (if controlPlaneRef.apiGroup=="controlplane.cluster.x-k8s.io" && controlPlaneRef.kind=="Kany8sControlPlane" then controlPlaneRef.name) ?? capiClusterName`
- `ackClusterName = Cluster.metadata.annotations["eks.kany8s.io/ack-cluster-name"] ?? eksClusterName`

注: BYO Topology サンプルでは ACK EKS Cluster 名が `Cluster.metadata.name` ではなく `Cluster.spec.controlPlaneRef.name` になる場合があるため、この既定値を採用する。

注: CAPI は `<capiClusterName>-kubeconfig` を読むため、Secret 名は `capiClusterName` を基準にします。

### AWS region の解決

token 生成（STS 署名）に region が必要です。
region は次の優先順で解決します。

1) `Cluster.metadata.annotations["eks.kany8s.io/region"]`（明示指定）
2) ACK EKS Cluster の `status.ackResourceMetadata.region`（同期後に確実に埋まる）
3) ACK EKS Cluster の `metadata.annotations["services.k8s.aws/region"]`（フォールバック）

上記で解決できない場合は NotReady 扱いで requeue し、Event で理由を出します。

### endpoint / CA data の取得

EKS kubeconfig には以下が必要です。

- API endpoint URL
- `certificate-authority-data`

MVP は Kubernetes API 経由（ACK）で取得します。

- `clusters.eks.services.k8s.aws/<ackClusterName>` の `status.endpoint` / `status.certificateAuthority.data`

注: ACK CRD の該当フィールドは `status.endpoint` / `status.certificateAuthority.data`。

### 出力: CAPI 互換 kubeconfig Secret

`<cluster>-kubeconfig` Secret を作成/更新します。

- `metadata.name`: `<cluster>-kubeconfig`
- `metadata.namespace`: `Cluster` と同一
- `metadata.labels["cluster.x-k8s.io/cluster-name"]`: `<cluster>`
- `type`: `cluster.x-k8s.io/secret`
- `data.value`: kubeconfig

kubeconfig の user は `exec` ではなく **token 埋め込み** を使います。

```yaml
apiVersion: v1
kind: Config
clusters:
- name: <cluster>
  cluster:
    server: https://...  # EKS endpoint
    certificate-authority-data: ...
users:
- name: aws
  user:
    token: k8s-aws-v1.... # 期限付き
contexts:
- name: <cluster>
  context:
    cluster: <cluster>
    user: aws
current-context: <cluster>
```

### 人間向け kubeconfig（MVP で生成）

利用者がローカルから `kubectl` する用途では `exec` kubeconfig の方が扱いやすいケースがあります。
MVP では probe 用 Secret とは別に、以下を生成します。

- `metadata.name: <cluster>-kubeconfig-exec`
- `data.value`: `exec` 付き kubeconfig（`aws eks get-token`）

※ これは CAPI probe 用 Secret と分離し、CAPI 側が参照する `<cluster>-kubeconfig` を汚さない。

## Secret の所有/競合ポリシー

`<cluster>-kubeconfig` は CAPI core が読むため、別コンポーネントと競合すると `RemoteConnectionProbe` が不安定になります。
そのため、本プラグインは以下のルールで Secret を管理します。

- 本プラグインが作成した Secret には `eks.kany8s.io/managed-by=eks-kubeconfig-rotator` を付与する
- `<cluster>-kubeconfig` が既に存在し、かつ上記 annotation が無い場合は **上書きしない**
  - 利用者が take over したい場合:
    - 方式A: Secret を削除する（次 reconcile で作られる）
    - 方式B: `Cluster` へ `eks.kany8s.io/allow-unmanaged-takeover=enabled` を付与し、明示的に in-place takeover を許可する
- OwnerReference を `Cluster` に付与し、`Cluster` 削除で Secret も GC されるようにする

## ローテーション設計

### 前提

- IAM token は短命なので “埋め込み kubeconfig” は定期更新が必須。
- CAPI ClusterCache は Unauthorized を検知すると切断して即 reconnect します。
  - `<cluster>-kubeconfig` が新 token に更新されていれば `RemoteConnectionProbe` は復帰できます。

### 更新タイミング

Secret に token の有効期限（および更新元情報）を annotation として保持し、残り時間に応じて再生成します。

例:

- `eks.kany8s.io/token-expiration-rfc3339: "2026-02-07T12:34:56Z"`
- `eks.kany8s.io/region: "ap-northeast-1"`
- `eks.kany8s.io/cluster-name: "demo"`

更新ポリシー（例）:

- 期限まで `<= 5m` なら更新
- それ以外でも “上限間隔” として `10m` で更新

これにより token 失効で 50 秒以上 probe が落ちる状況を避けやすくします。

## なぜ Option B (`status.kubeconfigSecretRef`) では足りないか

Kany8s facade の kubeconfig reconcile は `status.kubeconfigSecretRef` を見て source Secret から `<cluster>-kubeconfig` をコピーします。
しかし source Secret をローテーションしても、Kany8s facade がそれを watch して継続同期する仕組みは（現状）ありません。

EKS の token は短命なので、probe を安定させるには `<cluster>-kubeconfig` 自体を “定期的に更新” する必要があり、プラグインが直接 Secret を管理する方が自然です。

## RBAC（概略）

最低限:

- read:
  - `cluster.cluster.x-k8s.io` `clusters`
  - (ACK 経由なら) `eks.services.k8s.aws` `clusters`
- write:
  - core `secrets`（`<cluster>-kubeconfig` を作成/更新）
- events:
  - core `events`

## AWS 権限（概略）

MVP は ACK から endpoint/CA を読むため、EKS の `DescribeCluster` は必須ではありません。
token 生成は STS presign ベースで、実運用では `sts:GetCallerIdentity` が許可されている前提で扱います。

重要: 生成した token で Kubernetes API にアクセスできるかは「その token を発行する AWS principal が EKS 側で認可されているか」に依存します。
BYO サンプルでは ACK の `Cluster.spec.accessConfig.bootstrapClusterCreatorAdminPermissions: true` を使うため、
plugin が ACK と同じ AWS 資格情報（同一 principal）で token を生成すれば疎通しやすくなります。

実装済みの `config/eks-plugin/` manifest は `ack-system` namespace に配置し、
`ack-system/aws-creds` を `/var/run/secrets/aws/credentials` に mount する前提（ACK controller と同じ Secret/path 規約）にしています。

## 期待される効果

- `<cluster>-kubeconfig` が常に有効な token を含むため、CAPI の `RemoteConnectionProbe=True` が成立しやすくなる。
- その結果 `Cluster Available=True` の到達可能性が上がる（network 到達性がある場合）。

## BYO network サンプルへの適用

BYO サンプルで `Available=True` を狙う場合は、以下をセットで導入します。

- EKS ControlPlane 作成（kro + ACK）
- 本プラグイン（`eks-kubeconfig-rotator`）
- `Cluster` へ opt-in annotation（例: `eks.kany8s.io/kubeconfig-rotator=enabled`）

## オープン事項

- opt-in annotation を ClusterClass/Topology から注入するか（利用者 UX の統一）。
- multi-region を 1 controller で扱う場合の AWS SDK client/STS endpoint の作り方。
- `<cluster>-kubeconfig` を既存 Secret から take over する UX（削除方式で十分か、明示 allow が必要か）。
