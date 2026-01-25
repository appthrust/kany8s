# 09. API / 設定リファレンス(要点)

この章は「仕様を素早く引く」ための要約です。詳細は公式 API Reference も参照してください。

- RGD CRD: https://kro.run/api/crds/resourcegraphdefinition
- SimpleSchema: https://kro.run/api/specifications/simple-schema

## ResourceGraphDefinition (RGD)

### 基本

- apiVersion: `kro.run/v1alpha1`
- kind: `ResourceGraphDefinition`
- scope: Cluster

### spec.schema

生成 CRD の仕様(= 利用者が作るインスタンスの schema)。

- `apiVersion` (required, immutable)
  - 例: `v1alpha1`
- `kind` (required, immutable)
  - 例: `WebApplication`
- `group` (immutable, default `kro.run`)
- `spec`
  - SimpleSchema による spec 定義
- `status`
  - CEL で下位リソースから値を射影し、型は推論
- `types`
  - SimpleSchema のカスタム型
- `additionalPrinterColumns`
  - 生成 CRD の printer columns

### spec.resources[]

インスタンスが管理するリソース群。

- `id` (required)
- `template` または `externalRef` のどちらか一方(required)
- `includeWhen`: boolean CEL の配列
- `readyWhen`: boolean CEL の配列

#### externalRef

既存リソースを読み取るだけで、kro が lifecycle を管理しません。

- `apiVersion` (required)
- `kind` (required)
- `metadata.name` (required)
- `metadata.namespace` (optional; empty の場合はインスタンス namespace)

#### template

通常の Kubernetes マニフェストを記述し、値の一部に `${...}` を埋め込みます。

## RGD status

### status.state

RGD の状態を表します(例: Active/Inactive)。

### status.topologicalOrder

依存関係を元に算出した作成順序。

### status.conditions

RGD が受理/CRD 生成/コントローラ準備完了などを表す conditions。

0.7.1 の代表例:

- `ResourceGraphAccepted`
- `KindReady`
- `ControllerReady`

### status.resources[].dependencies

各 resource が依存している resource id の一覧。

## 生成 CRD(インスタンス側)で重要な仕様

- `status.conditions` と `status.state` は kro が予約/自動注入
- spec は Kubernetes API server の admission で検証される(= 不正なインスタンスは作れない)

## Helm chart values の要点(v0.7.1)

この節は `helm/values.yaml` を元に、運用で触りやすい値だけ抜粋します。

### rbac

- `rbac.mode: unrestricted|aggregation`

### config

- `config.allowCRDDeletion: false`
  - kro が CRD を削除することを許可するフラグ(デフォルト無効)
- `config.clientQps: 100`
- `config.clientBurst: 150`
- `config.enableLeaderElection: true`
- `config.leaderElectionNamespace: ""`
- `config.metricsBindAddress: :8078`
- `config.healthProbeBindAddress: :8079`
- `config.resourceGraphDefinitionConcurrentReconciles: 1`
- `config.dynamicControllerConcurrentReconciles: 1`
- `config.dynamicControllerDefaultResyncPeriod: 36000` (10h)
- `config.dynamicControllerDefaultQueueMaxRetries: 20`
- `config.logLevel: info`

### metrics

- `metrics.service.create: false`
- `metrics.service.port: 8080`
- `metrics.serviceMonitor.enabled: false`

### image

- `image.repository: registry.k8s.io/kro/kro`
- `image.tag: <chart appVersion>`
- `image.pullPolicy: IfNotPresent`
- `image.ko: false` (開発時に ko を使う)

### deployment

- `deployment.replicaCount: 1`
- `deployment.resources.requests/limits`
- `deployment.securityContext` (runAsNonRoot 等)

## 参照: インストール用マニフェスト

リリースの `kro-core-install-manifests*.yaml` には、CRD と controller の Deployment/ServiceAccount/ClusterRole 等が含まれます。

例:

```text
https://github.com/kubernetes-sigs/kro/releases/download/v0.7.1/kro-core-install-manifests.yaml
```

これを読むと「どの権限が入っているか」「どの env/args が渡っているか」を確認できます。
